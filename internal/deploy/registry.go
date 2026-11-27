package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/reference"
)

// ImageInfo is what an image's config says about running it, read from
// its registry without pulling it, for filling in the launch form.
type ImageInfo struct {
	Image string `json:"image"`
	// Container ports the image declares with EXPOSE
	Ports []int `json:"ports"`
	// Where the image keeps data (VOLUME), lost on redeploy unless mounted
	Volumes     []string `json:"volumes"`
	Healthcheck bool     `json:"healthcheck,omitempty"`
	// The port hostnames most likely route to
	Port int `json:"port,omitempty"`
}

var manifestTypes = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// InspectImage reads the config of an image from its registry. It works
// for public images and for the central registry.
func (d *Deployer) InspectImage(ctx context.Context, ref string) (*ImageInfo, error) {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return nil, fmt.Errorf("%q is not an image reference", ref)
	}
	named = reference.TagNameOnly(named)

	host := reference.Domain(named)
	if host == "docker.io" {
		host = "registry-1.docker.io"
	}
	version := "latest"
	if digested, ok := named.(reference.Digested); ok {
		version = digested.Digest().String()
	} else if tagged, ok := named.(reference.Tagged); ok {
		version = tagged.Tag()
	}

	r := &registryClient{
		http: &http.Client{Timeout: 15 * time.Second},
		base: "https://" + host + "/v2/" + reference.Path(named),
	}
	if d.cfg.RegistryUser != "" && host == d.registryHost() {
		r.user, r.password = d.cfg.RegistryUser, d.cfg.RegistryPassword
	}

	var m struct {
		MediaType string `json:"mediaType"`
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := r.get(ctx, "/manifests/"+version, manifestTypes, &m); err != nil {
		return nil, err
	}

	// A multi-platform image: any platform's config says the same about
	// ports and volumes, so prefer the usual one
	if len(m.Manifests) > 0 {
		pick := ""
		for _, entry := range m.Manifests {
			if entry.Platform.OS == "linux" && entry.Platform.Architecture == "amd64" {
				pick = entry.Digest
				break
			}
			if pick == "" && entry.Platform.OS != "unknown" {
				pick = entry.Digest
			}
		}
		if pick == "" {
			return nil, fmt.Errorf("the image has no runnable platform")
		}
		m.Manifests = nil
		if err := r.get(ctx, "/manifests/"+pick, manifestTypes, &m); err != nil {
			return nil, err
		}
	}
	if m.Config.Digest == "" {
		return nil, fmt.Errorf("unsupported manifest type %q", m.MediaType)
	}

	var config struct {
		Config struct {
			ExposedPorts map[string]struct{} `json:"ExposedPorts"`
			Volumes      map[string]struct{} `json:"Volumes"`
			Healthcheck  *struct {
				Test []string `json:"Test"`
			} `json:"Healthcheck"`
		} `json:"config"`
	}
	if err := r.get(ctx, "/blobs/"+m.Config.Digest, "*/*", &config); err != nil {
		return nil, err
	}

	info := &ImageInfo{Image: named.String(), Ports: []int{}, Volumes: []string{}}
	for spec := range config.Config.ExposedPorts {
		port, proto, _ := strings.Cut(spec, "/")
		if n, err := strconv.Atoi(port); err == nil && (proto == "" || proto == "tcp") {
			info.Ports = append(info.Ports, n)
		}
	}
	sort.Ints(info.Ports)
	for v := range config.Config.Volumes {
		info.Volumes = append(info.Volumes, v)
	}
	sort.Strings(info.Volumes)
	if hc := config.Config.Healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] != "NONE" {
		info.Healthcheck = true
	}
	if len(info.Ports) > 0 {
		ports := make([]StackPort, len(info.Ports))
		for i, p := range info.Ports {
			ports[i] = StackPort{Target: p}
		}
		info.Port = pickPort(ports)
	}
	return info, nil
}

type registryClient struct {
	http           *http.Client
	base           string
	user, password string
	token          string
}

// get fetches a registry path as JSON, answering a token challenge once.
func (r *registryClient) get(ctx context.Context, path, accept string, out any) error {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", accept)
		if r.token != "" {
			req.Header.Set("Authorization", "Bearer "+r.token)
		} else if r.user != "" {
			req.SetBasicAuth(r.user, r.password)
		}

		res, err := r.http.Do(req)
		if err != nil {
			return fmt.Errorf("couldn't reach the registry at %s", req.URL.Host)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		res.Body.Close()
		if err != nil {
			return err
		}

		switch {
		case res.StatusCode == http.StatusUnauthorized && attempt == 0 && r.token == "":
			if err := r.authenticate(ctx, res.Header.Get("WWW-Authenticate")); err != nil {
				return err
			}
			continue
		case res.StatusCode == http.StatusNotFound:
			return fmt.Errorf("the image or tag doesn't exist")
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			// Docker Hub answers this way for images that don't exist too
			return fmt.Errorf("the image doesn't exist, or is private")
		case res.StatusCode != http.StatusOK:
			return fmt.Errorf("the registry answered %s", res.Status)
		}
		return json.Unmarshal(body, out)
	}
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// authenticate gets a bearer token for a challenge like
// Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/postgres:pull"
func (r *registryClient) authenticate(ctx context.Context, challenge string) error {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "bearer") {
		return fmt.Errorf("the image doesn't exist, or is private")
	}
	values := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(params, -1) {
		values[m[1]] = m[2]
	}
	realm, err := url.Parse(values["realm"])
	if err != nil || realm.Scheme != "https" {
		return fmt.Errorf("the registry sent an unusable login challenge")
	}
	q := realm.Query()
	for _, key := range []string{"service", "scope"} {
		if values[key] != "" {
			q.Set(key, values[key])
		}
	}
	realm.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return err
	}
	if r.user != "" {
		req.SetBasicAuth(r.user, r.password)
	}
	res, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach the registry's login at %s", req.URL.Host)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the image doesn't exist, or is private")
	}
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&token); err != nil {
		return err
	}
	r.token = token.Token
	if r.token == "" {
		r.token = token.AccessToken
	}
	if r.token == "" {
		return fmt.Errorf("the registry sent no token")
	}
	return nil
}

// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/containers": {
            "get": {
                "description": "Get a list of all Docker containers",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "containers"
                ],
                "summary": "List all containers",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Server name",
                        "name": "server",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {}
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {}
                    }
                }
            }
        },
        "/containers/restart/{id}": {
            "post": {
                "description": "Restart a Docker container by ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "containers"
                ],
                "summary": "Restart a container",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Container ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Server name",
                        "name": "server",
                        "in": "query"
                    },
                    {
                        "type": "boolean",
                        "default": false,
                        "description": "Pull latest image before restart",
                        "name": "pull_latest",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {}
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {}
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {}
                    }
                }
            }
        },
        "/containers/start/{id}": {
            "post": {
                "description": "Start a Docker container by ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "containers"
                ],
                "summary": "Start a container",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Container ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Server name",
                        "name": "server",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {}
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {}
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {}
                    }
                }
            }
        },
        "/containers/stop/{id}": {
            "post": {
                "description": "Stop a Docker container by ID",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "containers"
                ],
                "summary": "Stop a container",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Container ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Server name",
                        "name": "server",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {}
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {}
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {}
                    }
                }
            }
        },
        "/containers/{id}": {
            "get": {
                "description": "Get detailed information about a specific Docker container",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "containers"
                ],
                "summary": "Get container details",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Container ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Server name",
                        "name": "server",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {}
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {}
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {}
                    }
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:3000",
	BasePath:         "/",
	Schemes:          []string{},
	Title:            "Docktrine API",
	Description:      "Docker management API",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}

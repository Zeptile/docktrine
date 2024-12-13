if ! command -v swag &> /dev/null; then
    echo "Error: 'swag' command not found."
    echo "Please install swaggo/swag first using:"
    echo "go install github.com/swaggo/swag/cmd/swag@latest"
    exit 1
fi

swag init -g cmd/api/main.go
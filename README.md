# Image Server
This is a simple image server written in Go. It uses AWS S3 for storage and caching.

## Features
- Image upload and retrieval
- Caching of images to improve performance
- Configurable settings for cache and storage
## Getting Started
### Prerequisites
- Go 1.16 or later
- AWS account with S3 access
### Installation
1. Clone the repository
    ```bash
    git clone https://github.com/yourusername/imageserver.git
    ```
2. Navigate to the project directory
    ```bash
    cd imageserver
    ```
3. Install dependencies
    ```bash
    go mod download
    ```
#### Configuration
set the following environment variables:
- `AWS_ACCESS_KEY_ID` - AWS access key ID
- `AWS_SECRET_ACCESS_KEY` - AWS secret access key
- `AWS_REGION` - AWS region
- `AWS_BUCKET` - AWS S3 bucket name


### Usage
Run the server with:
```bash
go run main.go
```

### Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

#### License
This project is licensed under the MIT License. See the `LICENSE` file for details.
## Docker Deployment


### Build and Run
```bash
# Build image
docker build -t l7-proxy:latest .

# Run container
docker run -d \
  -p 8080:8080 \
  -p 8081:8081 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  l7-proxy:latest
```
# Deployment Guide

This guide covers various deployment scenarios for the L7 load balancer.

## Prerequisites

- Go 1.25.4+ (for building from source)
- Linux/macOS/Windows (for deployment)
- Basic networking knowledge
- TLS certificates (for production deployment)

## Build from Source

### Development Build
```bash
# Clone repository
git clone https://github.com/yourusername/load-balancer.git
cd load-balancer

# Install dependencies
go mod download

# Build
go build -o l7-proxy ./cmd/proxy

# Run
./l7-proxy
```

### Production Build
```bash
# Build with optimizations
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w" \
  -o l7-proxy \
  ./cmd/proxy

# Verify build
./l7-proxy --version
```

## Docker Deployment

### Dockerfile
```dockerfile
# Multi-stage build for smaller image
FROM golang:1.25.4-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o l7-proxy ./cmd/proxy

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/l7-proxy .
COPY config.yaml .
EXPOSE 8080 8081
CMD ["./l7-proxy"]
```

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

### Docker Compose
```yaml
version: '3.8'
services:
  load-balancer:
    build: .
    ports:
      - "8080:8080"
      - "8081:8081"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./certs:/app/certs
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8081/metrics"]
      interval: 30s
      timeout: 10s
      retries: 3
```

## Kubernetes Deployment

### ConfigMap for Configuration
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: lb-config
data:
  config.yaml: |
    server:
      listen: ":8080"
      read_timeout: 30
      write_timeout: 30
      idle_timeout: 120
      admin_listen: ":8081"
    
    backends:
      - url: "http://backend-service:8080"
        weight: 1
    
    health:
      interval_seconds: 10
      timeout_seconds: 5
      path: "/health"
    
    rate_limit:
      rps: 100
      burst: 120
    
    strategy: "round_robin"
    fasthttp: true
```

### Secret for TLS Certificates
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: lb-tls
type: tls
data:
  tls.crt: <base64-encoded-cert>
  tls.key: <base64-encoded-key>
```

### Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: load-balancer
  labels:
    app: load-balancer
spec:
  replicas: 3
  selector:
    matchLabels:
      app: load-balancer
  template:
    metadata:
      labels:
        app: load-balancer
    spec:
      containers:
      - name: l7-proxy
        image: your-registry/l7-proxy:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 8081
          name: admin
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
        - name: tls
          mountPath: /app/certs
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /metrics
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /metrics
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: lb-config
      - name: tls
        secret:
          secretName: lb-tls
```

### Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: load-balancer
spec:
  selector:
    app: load-balancer
  ports:
  - port: 80
    targetPort: 8080
    name: http
  - port: 8081
    targetPort: 8081
    name: admin
  type: LoadBalancer
```

### Horizontal Pod Autoscaler
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: load-balancer-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: load-balancer
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

## Systemd Service (Linux)

### Service File
```ini
[Unit]
Description=L7 Load Balancer
After=network.target

[Service]
Type=simple
User=loadbalancer
Group=loadbalancer
WorkingDirectory=/opt/l7-proxy
ExecStart=/opt/l7-proxy/l7-proxy
Restart=always
RestartSec=5
LimitNOFILE=65536

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/l7-proxy

[Install]
WantedBy=multi-user.target
```

### Installation
```bash
# Create user
sudo useradd -r -s /bin/false loadbalancer

# Create directories
sudo mkdir -p /opt/l7-proxy /var/log/l7-proxy
sudo chown loadbalancer:loadbalancer /opt/l7-proxy /var/log/l7-proxy

# Copy binary and config
sudo cp l7-proxy /opt/l7-proxy/
sudo cp config.yaml /opt/l7-proxy/
sudo chown -R loadbalancer:loadbalancer /opt/l7-proxy

# Install service
sudo cp l7-proxy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable l7-proxy
sudo systemctl start l7-proxy
```

## Cloud Provider Deployment

### AWS
- **EC2**: Deploy on EC2 instances with ALB/ELB
- **EKS**: Use Kubernetes deployment
- **ECS**: Deploy with Docker containers
- **Lambda**: Consider serverless deployment (limited use case)

### GCP
- **Compute Engine**: Deploy on VM instances
- **GKE**: Use Kubernetes deployment
- **Cloud Run**: Serverless container deployment
- **Cloud Load Balancing**: Use as backend for GCLB

### Azure
- **Virtual Machines**: Deploy on Azure VMs
- **AKS**: Use Kubernetes deployment
- **Container Instances**: Container-based deployment
- **Application Gateway**: Use as backend

## Configuration Management

### Environment Variables
```bash
# Override configuration with environment variables
export LB_SERVER_LISTEN=":8080"
export LB_RATE_LIMIT_RPS="200"
export LB_STRATEGY="least_conn"
./l7-proxy
```

### Hot Reload
The load balancer supports configuration hot-reload:
```bash
# Signal USR1 to reload configuration
kill -USR1 <pid>

# Or use the admin API
curl -X POST http://localhost:8081/admin/reload
```

## Monitoring Integration

### Prometheus Scraping
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'load-balancer'
    static_configs:
      - targets: ['localhost:8081']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### Grafana Dashboard
Import the provided Grafana dashboard for load balancer metrics visualization.

### Log Aggregation
```yaml
# Filebeat configuration
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/l7-proxy/*.log
  json.keys_under_root: true
  json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

## Performance Tuning

### System Limits
```bash
# Increase file descriptor limit
ulimit -n 65536

# Add to /etc/security/limits.conf
* soft nofile 65536
* hard nofile 65536
```

### Network Tuning
```bash
# TCP settings
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
sysctl -w net.core.netdev_max_backlog=65535

# Make persistent in /etc/sysctl.conf
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.core.netdev_max_backlog = 65535
```

### Go Runtime Tuning
```bash
# Set GOMAXPROCS
export GOMAXPROCS=$(nproc)

# Memory limits
export GOMEMLIMIT=512MiB
```

## Security Hardening

### TLS Configuration
```yaml
tls:
  enabled: true
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"
  min_version: "TLS1.2"
  max_version: "TLS1.3"
```

### Firewall Rules
```bash
# Allow HTTP/HTTPS
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Allow admin port from trusted IPs only
iptables -A INPUT -p tcp -s 10.0.0.0/8 --dport 8081 -j ACCEPT

# Drop everything else
iptables -A INPUT -j DROP
```

### SELinux/AppArmor
Configure SELinux or AppArmor profiles for additional security.

## Troubleshooting

### Common Issues

**High Memory Usage**
- Check connection pool configuration
- Monitor metrics for memory leaks
- Adjust buffer pool sizes

**Connection Timeouts**
- Verify backend health
- Check network connectivity
- Adjust timeout values

**High CPU Usage**
- Check for TLS overhead
- Monitor TLS handshake times
- Consider disabling HTTP/2 if not needed

### Debug Mode
```bash
# Enable debug logging
export LOG_LEVEL=debug
./l7-proxy

# Enable pprof
curl http://localhost:8081/debug/pprof/goroutine?debug=1
```

## Backup and Recovery

### Configuration Backup
```bash
# Backup configuration
cp config.yaml config.yaml.backup
tar -czf config-backup-$(date +%Y%m%d).tar.gz config.yaml certs/

# Restore
tar -xzf config-backup-20241201.tar.gz
```

### State Backup
The load balancer is stateless, but you may want to backup:
- Configuration files
- TLS certificates
- Prometheus metrics data
- Log files

## Upgrade Strategy

### Rolling Update
```bash
# Kubernetes rolling update
kubectl set image deployment/load-balancer l7-proxy=l7-proxy:v2.0.0

# Docker rolling update
docker-compose up -d --no-deps --build load-balancer
```

### Blue-Green Deployment
Maintain two identical environments and switch traffic between them.

### Canary Deployment
Deploy new version to subset of instances and monitor before full rollout.

## Disaster Recovery

### High Availability
- Deploy multiple instances behind DNS round-robin
- Use health checks for automatic failover
- Consider active-passive setup with VIP

### Backup Locations
- Store configuration in version control
- Backup TLS certificates securely
- Keep recent binary versions available
- Document deployment procedures

## Performance Testing

### Load Testing
```bash
# Initial load test
hey -z 60s -c 100 http://load-balancer:8080/

# Stress test
hey -z 300s -c 1000 http://load-balancer:8080/

# Sustained load test
hey -z 3600s -c 500 http://load-balancer:8080/
```

### Benchmark Suite
```bash
# Run full benchmark suite
make bench-full

# Analyze results
# Review P50, P95, P99 latencies
# Check error rates
# Monitor resource usage
```

## Support and Maintenance

### Monitoring Alerts
Set up alerts for:
- Error rate > 1%
- P95 latency > 100ms
- Backend health < 50%
- CPU > 80%
- Memory > 80%

### Regular Maintenance
- Update dependencies monthly
- Review logs weekly
- Check SSL certificate expiration
- Update configuration as needed
- Monitor security advisories

### Log Rotation
```bash
# Configure logrotate
cat > /etc/logrotate.d/l7-proxy << EOF
/var/log/l7-proxy/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 loadbalancer loadbalancer
}
EOF
```

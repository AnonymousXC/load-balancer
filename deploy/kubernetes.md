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

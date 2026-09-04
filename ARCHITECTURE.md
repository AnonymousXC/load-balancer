# Architecture Documentation

## System Overview

This L7 load balancer is designed as a high-performance, production-grade proxy that operates at the application layer (Layer 7) of the OSI model. It provides intelligent request routing, resilience patterns, and comprehensive observability.

## Core Components

### 1. Load Balancing Engine

The load balancing engine is the heart of the system, responsible for distributing incoming requests across backend servers.

#### Architecture
```
┌─────────────────────────────────────────┐
│         Load Balancing Engine            │
│  ┌───────────────────────────────────┐  │
│  │      Strategy Selector            │  │
│  │  • Round Robin                    │  │
│  │  • Least Connections              │  │
│  │  • Consistent Hashing             │  │
│  │  • Weighted Round Robin           │  │
│  └───────────────────────────────────┘  │
│  ┌───────────────────────────────────┐  │
│  │      Backend Pool Manager         │  │
│  │  • Health Tracking                │  │
│  │  • Connection Counting             │  │
│  │  • Weight Management               │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

#### Strategy Implementations

**Round Robin**
- Simple, predictable distribution
- Atomic counter for thread safety
- O(1) time complexity
- Best for homogeneous backends

**Least Connections**
- Tracks active connections per backend
- Routes to least loaded backend
- Suitable for varying request processing times
- O(n) time complexity where n = number of backends

**Consistent Hashing**
- Hash-based routing for session affinity
- Minimal disruption on backend changes
- Uses virtual nodes for better distribution
- Consistent even during backend failures

**Weighted Round Robin**
- Considers backend capacity/weight
- Smooth distribution based on weights
- Atomic operations for thread safety
- Ideal for heterogeneous infrastructure

### 2. Resilience Layer

The resilience layer ensures system stability under adverse conditions.

#### Circuit Breaker Pattern
```
States: CLOSED → OPEN → HALF-OPEN → CLOSED

CLOSED: Requests pass through normally
OPEN: Requests fail fast (circuit open)
HALF-OPEN: Limited requests to test recovery
```

**Configuration**
- `MaxRequests`: Maximum requests in half-open state
- `Interval`: Rolling window for error counting
- `Timeout`: Time before transitioning from open to half-open
- `ReadyToTrip`: Custom function to determine when to trip

**Benefits**
- Prevents cascading failures
- Fast failure when backend is unhealthy
- Automatic recovery detection
- Configurable thresholds per backend

#### Retry Logic
- Exponential backoff with jitter
- Configurable retry attempts
- Retryable error classification
- Context-aware cancellation

### 3. Connection Management

#### HTTP Client Pool
```go
Connection Pool Configuration:
- MaxIdleConns: 10,000 (total)
- MaxIdleConnsPerHost: 1,000
- MaxConnsPerHost: 10,000
- IdleConnTimeout: 90s
- TLSHandshakeTimeout: 10s
- ForceAttemptHTTP2: true
```

#### fasthttp Optimization
- Zero-allocation request/response handling
- Custom byte buffer pools
- Optimized header copying
- Dual-stack DNS resolution

### 4. Security Layer

#### Middleware Chain
```
Request → Rate Limit → IP Filter → Size Limit → Security Headers → CORS → App
```

#### Rate Limiting
- Token bucket algorithm
- Per-IP rate limiting
- Configurable burst capacity
- Sliding window approximation

#### IP Filtering
- CIDR notation support
- Whitelist/blacklist mode
- Trusted proxy handling
- X-Forwarded-For parsing

#### Size Limits
- Request size validation
- Response size limits
- Memory protection
- Configurable thresholds

### 5. Observability Stack

#### Metrics Collection
```
Prometheus Metrics Categories:
- Counter: Request counts, error counts
- Gauge: Active connections, backend health
- Histogram: Latency percentiles, size distributions
- Summary: Aggregated statistics
```

#### Tracing
- Request ID generation (UUID-based)
- Context propagation
- Distributed tracing support
- Correlation with logs

#### Logging
- Structured JSON logging
- Request lifecycle logging
- Error context capture
- Sampling for high throughput

## Data Flow

### Request Processing Flow
```
1. Request Reception
   └─> TCP connection accepted
   └─> HTTP request parsed

2. Security Layer
   └─> Rate limit check
   └─> IP filter validation
   └─> Size limit check
   └─> Security headers injection

3. Request Processing
   └─> Request ID generation
   └─> Logging initialization
   └─> Context creation

4. Load Balancing
   └─> Strategy selection
   └─> Backend selection
   └─> Circuit breaker check
   └─> Health verification

5. Proxying
   └─> Connection pool lookup
   └─> Request forwarding
   └─> Response handling
   └─> Retry on failure

6. Response Processing
   └─> Metrics collection
   └─> Logging completion
   └─> Response headers injection
   └─> Response delivery
```

## Concurrency Model

### Goroutine Usage
- **Main goroutine**: Server lifecycle management
- **Per-connection goroutines**: Request handling (net/http) or event loop (fasthttp)
- **Health check goroutines**: Periodic backend health monitoring
- **Config watcher goroutine**: Configuration hot-reload
- **Metrics collection**: In-place operations (atomic)

### Synchronization Primitives
- **sync.RWMutex**: Backend pool access
- **sync.Pool**: Object pooling (buffers, byte slices)
- **atomic.Int64**: Connection counters
- **atomic.Uint64**: Round-robin index
- **channels**: Graceful shutdown coordination

## Memory Management

### Object Pooling
```
Buffer Pool:
- Reused byte buffers
- Size-based pools
- Automatic cleanup
- Reduces GC pressure

Byte Pool:
- Fixed-size byte slices
- Multi-size pools
- Efficient reuse
- Zero-alloc hot paths
```

### Memory Optimization Strategies
1. **Object Reuse**: sync.Pool for frequently allocated objects
2. **Zero-Copy**: Minimize memory copies in hot paths
3. **Buffer Reuse**: Reuse HTTP buffers
4. **Efficient Structures**: Compact data structures
5. **GC Tuning**: Reduce allocation pressure

## Scaling Characteristics

### Horizontal Scaling
- **Stateless Design**: No shared state between instances
- **Configuration**: External configuration files
- **Session Affinity**: Via consistent hashing
- **Health Independent**: Each instance monitors backends independently

### Vertical Scaling
- **CPU Efficient**: Minimal blocking operations
- **Memory Efficient**: Object pooling and reuse
- **I/O Efficient**: Connection pooling and multiplexing
- **Network Efficient**: HTTP/2 and keep-alive

## Performance Considerations

### Critical Path Optimizations
1. **Fast Path**: Minimize operations in request handling
2. **Lock-Free**: Use atomic operations where possible
3. **Memory Pre-allocation**: Pre-allocate buffers
4. **System Calls**: Minimize system call overhead
5. **Context Switching**: Reduce goroutine context switches

### Bottleneck Analysis
- **Network**: I/O bound, optimize for throughput
- **CPU**: Compute bound during TLS and parsing
- **Memory**: GC pressure from allocations
- **Disk**: Minimal disk I/O (config only)

## Deployment Patterns

### Single Instance
- Development/testing
- Small workloads
- Simple deployments

### High Availability
- Multiple instances behind DNS round-robin
- Health checking for instance failover
- Shared configuration (Consul, etcd)

### Cloud Native
- Kubernetes deployment
- Service mesh integration
- Auto-scaling based on metrics
- Configuration via ConfigMaps/Secrets

## Monitoring & Alerting

### Key Metrics to Monitor
- Request rate and error rate
- Latency percentiles (p50, p95, p99)
- Backend health status
- Connection pool utilization
- Circuit breaker state changes
- Memory and CPU usage

### Alerting Thresholds
- Error rate > 1% for 5 minutes
- P95 latency > 100ms for 5 minutes
- Backend health < 50%
- Connection pool > 80% utilization
- Circuit breaker > 3 trips per minute

## Security Architecture

### Defense in Depth
1. **Network Level**: IP filtering, rate limiting
2. **Application Level**: CORS, security headers
3. **Transport Level**: TLS encryption
4. **Data Level**: Size limits, validation

### Threat Mitigation
- **DDoS**: Rate limiting, IP filtering
- **Injection**: Input validation, size limits
- **Data Exfiltration**: Response size limits
- **Man-in-the-Middle**: TLS, certificate validation

## Testing Strategy

### Unit Tests
- Individual component testing
- Mock dependencies
- Fast execution
- High coverage

### Integration Tests
- Component interaction testing
- Real dependencies (backends)
- Configuration validation
- Error scenario testing

### Load Tests
- Performance validation
- Stress testing
- Resource leak detection
- Benchmark regression

## Future Enhancements

### Planned Features
- gRPC support
- WebSocket proxying
- Request/response transformation
- Advanced rate limiting (sliding window)
- Service discovery integration
- Dynamic configuration API
- Traffic mirroring/shadowing
- Advanced routing rules
- WebAssembly plugins

### Research Areas
- Machine learning for load balancing
- Predictive auto-scaling
- Anomaly detection
- Automated configuration tuning

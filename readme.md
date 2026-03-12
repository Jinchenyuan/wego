### Project Overview
This is a one-stop microservices project based on **go-micro**, covering both development and deployment. It is suitable for lightweight mobile apps or small game backends.

If you want to use it:
- The **api** and **service** directories contain the actual business logic code — feel free to reuse or extract them as needed.
- All other code serves as the foundational infrastructure for the project.

### Tech Stack

#### Framework
- Reverse proxy: **Traefik**
- API gateway / HTTP handler: **Gin**
- Microservices framework: **go-micro**

#### Drivers
- Database: **pgx** (PostgreSQL driver)
- Cache: **go-redis** (Redis client)

#### Middleware & Components
- Cache: **Redis**
- Database: **PostgreSQL**
- Service discovery & configuration: **etcd**

### Deployment
- Containerization: **Docker images**
- Orchestration: **Kubernetes** (container management)

### Implemented Modules
- [x] **API**
- [x] **account** service

### Testing
1. **Local debugging**: Directly run the configurations in `launch.json` on your local machine using VSCode
2. **Local Kubernetes testing**: Use **k3d** or **minikube**

### Required Go Tool
```bash
go install github.com/micro/micro/v5/cmd/protoc-gen-micro@latest
```


<div align="center">
  <h1>🚀 ShopSwift - Microservices Backend</h1>
  <p><strong>Scalable, production-ready e-commerce backend built with Go and modern microservices architecture</strong></p>
  
  ![Go](https://img.shields.io/badge/Go-100%25-00ADD8?style=flat&logo=go&logoColor=white)
![Commits](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/yashrajoria/68669c1c711655a41895753490af2898/raw/commits-badge.json)
![Status](https://img.shields.io/badge/Status-Active-success?style=flat)

### 🛠️ Tech Stack

![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?logo=go&logoColor=white)
![Microservices](https://img.shields.io/badge/Architecture-Microservices-blue)
![REST API](https://img.shields.io/badge/API-RESTful-green)
![PostgreSQL](https://img.shields.io/badge/DB-PostgreSQL-336791?logo=postgresql&logoColor=white)
![MongoDB](https://img.shields.io/badge/DB-MongoDB-47A248?logo=mongodb&logoColor=white)
![Docker](https://img.shields.io/badge/Deploy-Docker-2496ED?logo=docker&logoColor=white)
![Kubernetes](https://img.shields.io/badge/Deploy-Kubernetes-326CE5?logo=kubernetes&logoColor=white)
</div>

---

## 📋 Overview

This repository contains the microservices-based backend for ShopSwift, an enterprise-grade e-commerce platform. The system is designed for high availability, scalability, and maintainability, leveraging a distributed architecture with independent services.

The backend is built entirely in **Go**, providing superior performance, efficient concurrency handling, and a robust standard library for building distributed systems.

---

## 🏗️ Architecture

The backend follows a microservices architectural pattern, consisting of:

### **API Gateway**
The entry point for all client requests, responsible for:
- Request routing to appropriate services
- Authentication and authorization
- Rate limiting and request validation
- Centralized logging and monitoring

### **Core Services**
- **BFF (Backend-for-Frontend):** Optimized aggregation layer with Redis-backed idempotency.
- **Auth Service:** Manages user authentication, registration, and session tokens.
- **Promotion Service:** Distributed coupon management with usage limit enforcement.
- **Shipping Service:** Dynamic zone-based rate calculation (Local & Cloud ready).
- **Cart Service:** Manages shopping cart state and validation.
- **Product Service:** Manages product catalog, categories, and inventory data.
- **Order Service:** Handles order placement, tracking, and history.
- **Common:** Shared libraries, utilities, and middleware.

---

## ✨ Key Features

- **Distributed System:** Independent services that can be scaled and deployed separately.
- **Idempotent Checkout:** Atomic Redis-backed logic to prevent duplicate orders.
- **Dynamic Shipping:** Intelligent zone-based rate calculation without external costs.
- **Atomic Promotions:** Safe coupon usage tracking across distributed systems.
- **API Gateway Routing:** Centralized entry point with optimized request handling.
- **High Performance:** Leverages Go's efficient runtime and concurrency primitives.

---

## 🛠️ Tech Stack

- **Language:** Go (Golang)
- **Architecture:** Microservices
- **Communication:** RESTful APIs
- **Databases Supported:**
  - PostgreSQL (Relational data)
  - MongoDB (Document data)
- **Deployment:** Docker & Kubernetes ready
- **Tooling:**
  - Standard Go toolchain
  - GDB for debugging
  - Version control with Git

---

## 📁 Project Structure

```
E-Commerce-backend/
└── backend/
    ├── api-gateway/         # Central API entry point and router
    └── services/            # Microservices directory
        ├── auth-service/    # Authentication and identity management
        ├── common/          # Shared utilities and middleware
        ├── inventory-service/# Real-time stock and inventory
        ├── order-service/   # Order processing and management
        ├── product-service/ # Product catalog and management
        └── user-service/    # User profiles and account data
```

---

## 🚀 Getting Started

### Prerequisites
- Go 1.20 or higher
- PostgreSQL & MongoDB
- Git

### Installation

1. **Clone the repository**
```bash
git clone https://github.com/yashrajoria/E-Commerce-backend.git
cd E-Commerce-backend
```

2. **Navigate to a specific service**
```bash
cd backend/services/auth-service
```

3. **Install dependencies**
```bash
go mod download
```

### Running Locally

Each service can be run independently:

```bash
go run main.go
```

To run the entire system, it's recommended to start the services in order (Auth, Products, etc.) and then start the API Gateway.

---

## 🔐 Security

The backend implements several security best practices:
- Centralized authentication via Auth Service
- Password hashing with validation
- Secure middleware for protected routes
- Request validation and sanitization
- Environment variable management for sensitive data

---

## 🚧 Future Roadmap

- [ ] Implementation of gRPC for inter-service communication
- [ ] Centralized configuration management
- [ ] Service discovery with Consul or Etcd
- [ ] Message queue integration (RabbitMQ/Kafka) for async tasks
- [ ] Comprehensive unit and integration test suite
- [ ] Prometheus and Grafana for metrics and monitoring
- [ ] ELK stack for centralized logging
- [ ] Distributed tracing with Jaeger

---

## 📝 Development Notes

### Technical Decisions
- **Go Selection:** Chosen for its performance, type safety, and first-class support for networking and concurrency.
- **Microservices vs Monolith:** Adopted to allow independent scaling and development of different domains.
- **RESTful Design:** Provides a standardized and well-documented interface for frontend applications.

### Performance
- Minimal runtime overhead
- Efficient memory management
- Fast startup times for services
- High throughput handling

---

## 🤝 Contributing

Contributions are welcome! This project has over 100+ commits and is actively evolving.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 👨‍💻 Author

**Yash Rajoria**
- GitHub: [@yashrajoria](https://github.com/yashrajoria)
- LinkedIn: [yashrajoria](https://www.linkedin.com/in/yashrajoria)

---

<div align="center">
  <p><strong>⭐ Star this repository if you find it helpful!</strong></p>
  <p>Made with ❤️ by Yash Rajoria</p>
</div>

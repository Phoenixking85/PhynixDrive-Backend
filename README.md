# PhynixDrive

The Ultimate Distributed Cloud Storage Backend

Enterprise-grade distributed file storage with real-time collaboration, built for scale

Features • Quick Start • API Documentation • Architecture • Contributing

---

## Overview

PhynixDrive is a **production-ready, distributed cloud storage backend** that combines the best of modern cloud technologies. Built with Go for performance and MongoDB for flexible data management, it leverages Backblaze B2's cost-effective storage and Google's robust OAuth system.

### Why PhynixDrive?

- Blazing Fast: Go-powered backend with optimized database queries
- Cost Effective: Backblaze B2 storage at fraction of traditional cloud costs
- Enterprise Security: Google OAuth + JWT with granular permissions
- Collaboration Ready: Real-time sharing with role-based access control
- Infinitely Scalable: Distributed architecture that grows with your needs
- Smart Search: Full-text search across files and folders
- Mobile Ready: RESTful API perfect for web, mobile, and desktop apps

---

## Features

### Authentication & Security

- Google OAuth 2.0 integration with secure JWT tokens
- Role-based permissions (Viewer, Editor, Admin)
- Secure file access with time-limited download URLs
- Comprehensive audit logging for all user actions

### File & Folder Management

- Nested folder structure with unlimited depth
- Drag-and-drop uploads with metadata preservation
- Version control for file history tracking
- Soft delete with trash bin and restore functionality
- Bulk operations for efficient file management

### Collaboration & Sharing

- Real-time collaboration with instant permission updates
- Granular sharing controls per file and folder
- Email notifications for sharing and updates
- Permission inheritance from parent folders

### Advanced Features

- Full-text search across filenames and folders
- Prometheus metrics for monitoring and observability
- Smart caching for improved performance
- Background processing for large file operations

---

## Quick Start

### Prerequisites

- **Go 1.21+** installed on your machine
- **MongoDB** database (local or cloud)
- **Backblaze B2** account with bucket setup
- **Google Cloud** project with OAuth credentials

### Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/phynixdrive.git
cd phynixdrive

# Install dependencies
go mod tidy

# Copy environment template
cp .env.example .env

# Edit your environment variables
nano .env
```

### Environment Setup

Create your `.env` file with the following configuration:

```env
# Server Configuration
PORT=8080
ENV=development

# MongoDB Configuration
MONGODB_URI=mongodb+srv://user:pass@cluster.mongodb.net/phynixdrive
MONGODB_DATABASE=phynixdrive

# Google OAuth Configuration
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-min-32-chars
JWT_EXPIRATION=24h
JWT_REFRESH_SECRET=your-refresh-token-secret-min-32-chars
JWT_REFRESH_EXPIRATION=7d

# Backblaze B2 Configuration
B2_KEY_ID=your-backblaze-key-id
B2_APP_KEY=your-backblaze-application-key
B2_BUCKET_NAME=your-bucket-name
B2_BUCKET_ID=your-bucket-id
B2_ENDPOINT=https://s3.us-west-000.backblazeb2.com

# Email Notifications (Optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASS=your-app-password

# Redis (Optional - for caching)
REDIS_URL=redis://localhost:6379
```

### Running the Application

```bash
# Development mode with hot reload
go run cmd/main.go

# Production build
go build -o bin/phynixdrive cmd/main.go
./bin/phynixdrive
```

Your API will be available at `http://localhost:8080`

---

## API Reference

### Authentication Endpoints

| Method | Endpoint             | Description                          |
|--------|----------------------|--------------------------------------|
| `GET`  | `/auth/oauth-url`    | Generate Google OAuth login URL       |
| `GET`  | `/auth/oauth-callback` | Handle Google OAuth callback & issue JWT |
| `POST` | `/auth/oauth-login`  | Login with Google OAuth token (direct flow) |
| `GET`  | `/auth/me`           | Get current user profile             |
| `POST` | `/auth/refresh`      | Refresh JWT token                    |
| `POST` | `/auth/logout`       | Logout and invalidate tokens         |


### Folder Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/folders` | List root folders |
| `POST` | `/folders` | Create new folder |
| `GET` | `/folders/:id` | Get folder details |
| `PUT` | `/folders/:id` | Update folder |
| `DELETE` | `/folders/:id` | Move folder to trash |
| `POST` | `/folders/:id/share` | Share folder with users |
| `GET` | `/folders/:id/permissions` | Get folder permissions |

### File Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/files` | List root files |
| `POST` | `/files` | Upload new file |
| `GET` | `/files/:id` | Get file metadata |
| `GET` | `/files/:id/download` | Download file |
| `PUT` | `/files/:id` | Update file metadata |
| `DELETE` | `/files/:id` | Move file to trash |
| `GET` | `/files/:id/versions` | Get file version history |
| `GET` | `/folders/:id/files` | List files in folder |

### Trash Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/trash` | List deleted items |
| `PATCH` | `/trash/:id/restore` | Restore from trash |
| `DELETE` | `/trash/:id/purge` | Permanently delete |

### Search & Discovery

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/search?q={query}` | Search files and folders |
| `GET` | `/search/recent` | Get recently accessed files |
| `GET` | `/search/shared` | Get shared files |

### Monitoring

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check endpoint |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/stats` | User statistics |

---

## Architecture

### Project Structure

```
phynixdrive/
├── 📁 cmd/                     # Application entry points
│   └── main.go
├── 📁 config/                  # Configuration management
│   └── config.go
├── 📁 controllers/             # HTTP request handlers
│   ├── auth_controller.go
│   ├── file_controller.go
│   ├── folder_controller.go
│   ├── trash_controller.go
│   └── search_controller.go
├── 📁 middleware/              # HTTP middleware
│   ├── auth_middleware.go
│   ├── permission_middleware.go
├── 📁 models/                  # Data models
│   ├── user.go
│   ├── file.go
│   ├── folder.go
│   ├── notification_log.go
|   |── trash.go
├── 📁 routes/                  # Route definitions
│   ├── auth_routes.go
│   ├── file_routes.go
│   ├── folder_routes.go
│   ├── trash_routes.go
│   ├── search_routes.go
│   └── router.go
├── 📁 services/                # Business logic layer
│   ├── auth_service.go
│   ├── b2_service.go
│   ├── file_service.go
│   ├── folder_service.go
│   ├── notification_service.go
│   ├── permission_service.go
│   └── search_service.go
│   └──trash_service.go

├── 📁 utils/                   # Utility functions
│   ├── response.go
│   ├── validation.go
│   ├── logger.go
│   └── jwt_utils.go

├── 📁 docs/                    # Documentation
│   ├── api/
│   └── deployment/
├── .env.example               # Environment template

├── go.mod                    # Go modules
└── README.md                 # This file
```

### Data Flow

The application follows a clean layered architecture:

1. **Client Request** → Auth Middleware validates JWT tokens
2. **Auth Middleware** → Controllers handle HTTP requests  
3. **Controllers** → Services contain business logic
- Google OAuth for authentication
- Backblaze B2 for file storage
- SMTP for email notifications
**External Integrations:**
- Google OAuth for authentication
- Backblaze B2 for file storage
#### Users Collection

### Database Schema

#### Users Collection
```json
{
  "_id": "ObjectId",
  "email": "user@example.com",
  "name": "John Doe",
  "avatar": "https://...",
  "googleId": "google-user-id",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z",
  "lastLogin": "2024-01-01T00:00:00Z",
  "storageUsed": 1073741824,
#### Files Collection
}
```

#### Files Collection
```json
{
  "_id": "ObjectId",
  "originalName": "document.pdf",
  "fileName": "uuid-filename.pdf",
  "mimeType": "application/pdf",
  "fileSize": 1048576,
  "b2Url": "https://...",
  "b2FileId": "b2-file-id",
  "ownerId": "ObjectId",
  "folderId": "ObjectId",
  "version": 1,
  "versions": [
    {
      "version": 1,
      "b2FileId": "b2-file-id",
      "uploadedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "sharedWith": [
    {
      "userId": "ObjectId",
      "role": "viewer",
      "sharedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "tags": ["important", "work"],
  "isDeleted": false,
  "deletedAt": null,
  "createdAt": "2024-01-01T00:00:00Z",
#### Folders Collection
}
```

#### Folders Collection
```json
{
  "_id": "ObjectId",
  "name": "My Documents",
  "ownerId": "ObjectId",
  "parentId": "ObjectId",
  "path": "/My Documents",
  "sharedWith": [
    {
      "userId": "ObjectId",
      "role": "editor",
      "sharedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "color": "#4285f4",
  "isDeleted": false,
  "deletedAt": null,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## Advanced Configuration

### Performance Tuning

```go
// Database connection pooling
mongoOptions := options.Client().ApplyURI(config.MongoURI).
    SetMaxPoolSize(100).
    SetMinPoolSize(10).
    SetMaxConnIdleTime(30 * time.Second)

// B2 client optimization
b2Client := &b2.Client{
    Timeout:    30 * time.Second,
    MaxRetries: 3,
    PartSize:   100 * 1024 * 1024, // 100MB parts
}
```

### Security Hardening

```go
// JWT Configuration
jwtConfig := &jwt.Config{
    SigningMethod: jwt.SigningMethodHS256,
    Expiration:    24 * time.Hour,
    RefreshExpiration: 7 * 24 * time.Hour,
    SecretKey:     []byte(config.JWTSecret),
}

// CORS Configuration
corsConfig := cors.Config{
    AllowOrigins:     []string{"https://yourdomain.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
}
```

### Monitoring & Logging

```go
// Structured logging with levels
logger := logrus.New()
logger.SetLevel(logrus.InfoLevel)
logger.SetFormatter(&logrus.JSONFormatter{})

// Prometheus metrics
var (
    uploadCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "phynixdrive_uploads_total",
            Help: "Total number of file uploads",
        },
        []string{"user_id", "status"},
    )
)
```

---

## 🧪 Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./tests/integration/...

# Run with race detection
go test -race ./...

# Benchmark tests
go test -bench=. ./...
```

### Test Structure

```go
func TestFileUpload(t *testing.T) {
    // Setup test database
    db := setupTestDB()
    defer cleanupTestDB(db)
    
    // Setup test services
    fileService := services.NewFileService(db, b2Client)
    
    // Test cases
    tests := []struct {
        name     string
        input    UploadRequest
        expected UploadResponse
        wantErr  bool
    }{
        // Test cases here
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

---

## Deployment

### Production Setup

1. **Server Requirements**
   - Linux server with Go 1.21+ installed
   - MongoDB database (local or cloud)
   ```bash
   # Build for production
   CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o phynixdrive cmd/main.go
   
   # Copy to server
   scp phynixdrive user@server:/opt/phynixdrive/
   scp .env user@server:/opt/phynixdrive/
   
   # Run on server
   chmod +x /opt/phynixdrive/phynixdrive
   nohup /opt/phynixdrive/phynixdrive &
   ```
   # Run on server
   chmod +x /opt/phynixdrive/phynixdrive
   ```bash
   # Using systemd (recommended)
   sudo nano /etc/systemd/system/phynixdrive.service
   
   # Service file content:
   [Unit]
   Description=PhynixDrive API Server
   After=network.target
   
   [Service]
   Type=simple
   User=www-data
   WorkingDirectory=/opt/phynixdrive
   ExecStart=/opt/phynixdrive/phynixdrive
   Restart=always
   RestartSec=5
   
   [Install]
   WantedBy=multi-user.target
   
   # Enable and start service
   sudo systemctl enable phynixdrive
   sudo systemctl start phynixdrive
   ```
   # Enable and start service
   sudo systemctl enable phynixdrive
   sudo systemctl start phynixdrive
   ```

### Reverse Proxy Setup

```nginx
# /etc/nginx/sites-available/phynixdrive
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com;
    
    ssl_certificate /path/to/certificate.crt;
    ssl_certificate_key /path/to/private.key;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # File upload size limit
        client_max_body_size 100M;
    }
}
```

### Production Checklist

- [ ] **Environment Variables**: All secrets properly configured
- [ ] **Database**: MongoDB with replica set for high availability
- [ ] **Storage**: Backblaze B2 bucket with proper CORS settings
- [ ] **Monitoring**: Prometheus + Grafana setup
- [ ] **Logging**: Centralized logging with ELK stack
- [ ] **SSL/TLS**: HTTPS certificates configured
- [ ] **Load Balancer**: Nginx or cloud load balancer setup
- [ ] **Backup**: Database backup strategy implemented
- [ ] **Security**: Security headers and rate limiting enabled

---

## Contributing

We welcome contributions! Please see our Contributing Guide for details.

### Reporting Issues

1. Search existing issues first
2. Use our issue templates
3. Provide detailed reproduction steps
4. Include system information

### Pull Request Process

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Commit changes: `git commit -m 'Add amazing feature'`
4. Push to branch: `git push origin feature/amazing-feature`
5. Open a Pull Request

### Development Guidelines

- Follow Go conventions and best practices
- Update documentation for API changes
- Use semantic commit messages
- Ensure code is properly formatted with `go fmt`

---

## License

This project is licensed under the MIT License - see the LICENSE file for details.

---

## Acknowledgments

- Google for OAuth 2.0 authentication
- Backblaze for affordable B2 cloud storage
- MongoDB for flexible document database
- Go Community for amazing ecosystem
- Contributors who make this project possible

- [Documentation](https://docs.phynixdrive.com)
- [API Reference](https://api.phynixdrive.com/docs)
- [Community](https://discord.gg/phynixdrive)
- [Blog](https://blog.phynixdrive.com)
- Documentation: https://docs.phynixdrive.com
- API Reference: https://api.phynixdrive.com/docs
- Community: https://discord.gg/phynixdrive
- Blog: https://blog.phynixdrive.com

Star us on GitHub • Follow us on Twitter • Join our Discord

Made with love by the PhynixDrive Team

Star us on GitHub • Follow us on Twitter • Join our Discord
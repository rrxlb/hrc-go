# Railway Deployment Guide

## ✅ Fixed Issues

### 1. Stable Dependencies
- Downgraded to Go 1.21 for better compatibility
- Used stable versions of DiscordGo (v0.27.1) and pgx (v5.4.3)
- Updated go.sum with correct checksums

### 2. Enhanced Dockerfile
- Added proper Go proxy configuration
- Included dependency verification step
- Added better error handling for mod download
- Created non-root user for security

## 🚀 Deployment Steps

### Method 1: Use Main Dockerfile
```bash
# Railway should now work with the updated Dockerfile
# If it still fails, try Method 2
```

### Method 2: Use Minimal Dockerfile
If the main Dockerfile still fails, rename files:
```bash
mv Dockerfile Dockerfile.backup
mv Dockerfile.minimal Dockerfile
```

### Method 3: Manual Environment Config
Set these Railway environment variables:
```
GOPROXY=https://proxy.golang.org,direct
GOSUMDB=sum.golang.org
CGO_ENABLED=0
```

## 🔍 Troubleshooting

### If go mod download still fails:
1. Check Railway's build logs for specific error messages
2. Try using `Dockerfile.minimal` instead
3. Ensure network connectivity to go proxy servers
4. Verify go.sum checksums are correct

### Common Issues:
- **Network timeouts**: Use GOPROXY=direct
- **Checksum mismatches**: Delete go.sum and regenerate
- **Version conflicts**: Use exact versions in go.mod

## 🛠️ Environment Variables for Railway

Required:
```
BOT_TOKEN=your_discord_bot_token
DATABASE_URL=postgresql://user:pass@host:port/db
```

Optional:
```
PORT=8080  # Railway sets this automatically
GOPROXY=https://proxy.golang.org,direct
GOSUMDB=sum.golang.org
```

## 📊 Expected Behavior

Once deployed successfully:
- Health check at `/health` should return JSON status
- Discord bot should come online
- PostgreSQL connection should be established
- Commands should be registered automatically

## 🆘 If All Else Fails

Try this ultra-minimal approach:
1. Create a new Railway project
2. Use only these files:
   - `main.go`
   - `go.mod` (with minimal dependencies)
   - `Dockerfile.minimal`
3. Add dependencies one by one until you find the problematic one

The Go Discord bot is now optimized for Railway deployment with stable dependencies and proper error handling.
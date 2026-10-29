> Gemini setup: configure `GEMINI_API_KEY` on the backend before starting. See [Gemini setup](README.md#gemini-setup) for current instructions.

# ====================================
# Daily Hikmah - Deployment Guide
# ====================================

## 🚀 Quick Start - Production Deployment

This guide will help you deploy Daily Hikmah to production using:
- **Render** (Go backend)
- **Vercel** (Static frontend)

---

## 📋 Prerequisites

1. **GitHub Account** (to connect repositories)
2. **Render Account** (https://render.com - free tier available)
3. **Vercel Account** (https://vercel.com - free tier available)

---

## STEP 1: Generate VAPID Keys

VAPID keys are required for Web Push authentication. Generate them once before deployment.

### Option A: Using the generator script

```bash
cd /Users/hassanomran/div/Ali
go run generate_vapid_keys.go
```

This will output your keys. **Save them securely!**

### Option B: Run backend once locally

```bash
go run .
```

The keys will be generated and printed to the console.

**⚠️ IMPORTANT**: Save these keys! You'll need them for Render environment variables.

---

## STEP 2: Deploy Backend to Render

### 2.1 Prepare Backend

1. **Rename production file**:
   ```bash
   mv main-production.go main.go
   ```

2. **Ensure `go.mod` and `go.sum` are up to date**:
   ```bash
   go mod tidy
   ```

3. **Push to GitHub** (create a new repo or use existing):
   ```bash
   git init
   git add .
   git commit -m "Prepare for production deployment"
   git remote add origin https://github.com/YOUR_USERNAME/daily-hikmah-backend.git
   git push -u origin main
   ```

### 2.2 Create Render Service

1. **Log in to Render** → https://dashboard.render.com

2. **Click "New +" → "Web Service"**

3. **Connect your GitHub repository**

4. **Configure the service**:
   - **Name**: `daily-hikmah-backend` (or your choice)
   - **Environment**: `Go`
   - **Build Command**: `go build -o app`
   - **Start Command**: `./app`
   - **Instance Type**: Free (or upgrade for production)

5. **Add Environment Variables** (⚠️ CRITICAL):
   - Click "Environment" tab
   - Add these variables:
     ```
     VAPID_PUBLIC_KEY=<your_public_key_from_step_1>
     VAPID_PRIVATE_KEY=<your_private_key_from_step_1>
     ```

6. **Click "Create Web Service"**

7. **Wait for deployment** (2-5 minutes)

8. **Copy your backend URL**:
   - It will look like: `https://daily-hikmah-backend.onrender.com`
   - **Save this URL!** You'll need it for the frontend.

---

## STEP 3: Deploy Frontend to Vercel

### 3.1 Update Frontend Configuration

1. **Edit `frontend/script.js`**:
   - Find line 8:
   ```javascript
   const API_URL = window.location.hostname === 'localhost' 
     ? 'http://localhost:8080'
     : 'https://YOUR_RENDER_BACKEND_URL_HERE.onrender.com';
   ```
   
   - Replace `YOUR_RENDER_BACKEND_URL_HERE.onrender.com` with your actual Render URL from Step 2.8
   
   Example:
   ```javascript
   const API_URL = window.location.hostname === 'localhost' 
     ? 'http://localhost:8080'
     : 'https://daily-hikmah-backend.onrender.com';
   ```

2. **Commit changes**:
   ```bash
   cd frontend
   git init
   git add .
   git commit -m "Configure production API URL"
   git remote add origin https://github.com/YOUR_USERNAME/daily-hikmah-frontend.git
   git push -u origin main
   ```

### 3.2 Deploy to Vercel

1. **Log in to Vercel** → https://vercel.com/dashboard

2. **Click "Add New..." → "Project"**

3. **Import your frontend repository**

4. **Configure the project**:
   - **Framework Preset**: Other (or leave as is)
   - **Root Directory**: `./` (or specify `frontend` if you pushed the whole repo)
   - **Build Command**: Leave empty (static site)
   - **Output Directory**: `./` (or leave empty)

5. **Click "Deploy"**

6. **Wait for deployment** (1-2 minutes)

7. **Your app is live!** 🎉
   - Vercel will give you a URL like: `https://daily-hikmah.vercel.app`

---

## STEP 4: Test the Production App

1. **Open your Vercel URL** in a browser

2. **Select a frequency** (6h, 12h, or 24h)

3. **Click "اشترك الآن" (Subscribe Now)**

4. **Allow notifications** when prompted

5. **You should receive an immediate welcome notification**

6. **Check Render logs** to see backend activity:
   - Go to Render Dashboard → Your Service → Logs
   - You should see: "New subscription added"

---

## 🔍 Troubleshooting

### Backend Issues

**Issue**: "VAPID keys not found in environment"
- **Solution**: Make sure you added `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` in Render environment variables

**Issue**: "Failed to load sayings"
- **Solution**: Set `GEMINI_API_KEY` in the backend environment

**Issue**: "Port already in use"
- **Solution**: This won't happen on Render (they assign a dynamic port). Only affects local development.

### Frontend Issues

**Issue**: "Failed to get server public key"
- **Solution**: Check that `API_URL` in `script.js` points to your Render backend URL

**Issue**: CORS errors in browser console
- **Solution**: Backend includes CORS middleware. Check that Render backend is running and accessible

**Issue**: Notifications not working
- **Solution**: 
  - Ensure you're using HTTPS (Vercel provides this automatically)
  - Check notification permissions in browser settings
  - Try in Incognito mode (clears cached permissions)

### General Issues

**Issue**: "Service Worker registration failed"
- **Solution**: Ensure `sw.js` is in the root of your frontend deployment

**Issue**: "403 Push service errors"
- **Solution**: 
  - VAPID keys might have changed
  - Clear browser cache and re-subscribe
  - Check Render logs for error details

---

## 📊 Monitoring

### Render Dashboard
- Monitor backend logs
- Check subscriber count via `/health` endpoint
- View memory and CPU usage

### Browser DevTools
- Console: Check for JavaScript errors
- Application → Service Workers: Verify SW is active
- Application → Push Messaging: Check subscription status

---

## 🔐 Security Best Practices

1. **Never commit VAPID keys to git**
   - Add `.env` to `.gitignore`
   - Use environment variables on Render

2. **Limit CORS origins in production** (optional):
   - Edit `corsMiddleware` in `main.go`
   - Replace `*` with your Vercel domain

3. **Use HTTPS everywhere**
   - Both Render and Vercel provide HTTPS by default

4. **Regularly update dependencies**:
   ```bash
   go get -u ./...
   go mod tidy
   ```

---

## 🎯 Next Steps

### Production Enhancements

1. **Add a database** (PostgreSQL on Render):
   - Store subscriptions persistently
   - Track notification history
   - Add user preferences

2. **Add analytics**:
   - Monitor subscription rates
   - Track notification delivery
   - User engagement metrics

3. **Custom domain**:
   - Add your own domain to Vercel
   - Update CORS settings accordingly

4. **Email notifications** (fallback):
   - Use SendGrid or similar
   - Notify users if push fails

5. **Admin dashboard**:
   - Add/edit wisdom sayings
   - View subscriber stats
   - Manually trigger notifications

---

## 📞 Need Help?

Check the main README.md for additional documentation and troubleshooting tips.

---

**🎉 Congratulations! Your Daily Hikmah app is now live in production!**

// ====================================
// Configuration
// ====================================

// IMPORTANT: Update RENDER_BACKEND_URL before deploying to Vercel
// Get your Render backend URL from: https://dashboard.render.com
// Example: 'https://daily-hikmah-backend.onrender.com'
const RENDER_BACKEND_URL = 'https://daily-hikmah-backend.onrender.com'; // ✅ UPDATED!

// Auto-detect environment: 
// - localhost OR onrender.com (backend serves frontend) → use same origin (empty string)
// - Vercel deployment → use RENDER_BACKEND_URL
const API_URL = window.location.hostname === 'localhost' 
  || window.location.hostname === '127.0.0.1'
  || window.location.hostname.includes('onrender.com')
  ? '' // Empty string = same origin (backend serves frontend)
  : RENDER_BACKEND_URL; // Production: Vercel frontend → Render backend

console.log('🌐 Environment:', window.location.hostname);
console.log('🔗 API URL:', API_URL || 'Same origin (localhost)');

if (API_URL.includes('YOUR_APP_NAME')) {
  console.error('⚠️ WARNING: Update RENDER_BACKEND_URL in script.js before deploying to Vercel!');
}

// ====================================
// DOM Elements
// ====================================

const subscribeBtn = document.getElementById("subscribeBtn");
const btnText = document.getElementById("btnText");
const unsubscribeBtn = document.getElementById("unsubscribeBtn");
const unsubBtnText = document.getElementById("unsubBtnText");
const statusEl = document.getElementById("status");
const wisdomEl = document.getElementById("wisdomText");
const frequencySection = document.getElementById("frequencySection");
const infoSection = document.getElementById("infoSection");
const countdownEl = document.getElementById("countdown");

let vapidPublicKey = null;
let countdownInterval = null;
let wisdomRefreshTimeout = null;

// ====================================
// Utility Functions
// ====================================

// Convert base64 string to Uint8Array for VAPID key
function urlBase64ToUint8Array(base64String) {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding)
    .replace(/-/g, "+")
    .replace(/_/g, "/");

  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(rawData.length);

  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

// Update status message
function updateStatus(message, isError = false) {
    statusEl.style.cssText = `
        font-size: 1rem;
        color: var(--primary-dark);
        background: #e9f0e4;
        padding: 0.6rem 1.8rem;
        border-radius: 50px;
        min-height: 3rem;
        text-align: center;
        border-right: 3px solid var(--secondary-color);
        max-width: 100%;
        word-break: break-word;
        `;
  statusEl.textContent = message;
  statusEl.className = isError ? "status error" : "status success";
  statusEl.style.display = "block";
}

// Get selected frequency from radio buttons
function getSelectedFrequency() {
  const selected = document.querySelector('input[name="frequency"]:checked');
  return selected ? parseInt(selected.value) : 24; // Default to 24 hours
}

// ====================================
// API Functions
// ====================================

// Load today's wisdom from backend
async function loadTodaysWisdom() {
  try {
    const response = await fetch(API_URL + "/api/hikmah");
    if (response.status === 503) {
      wisdomEl.textContent = "الحكمة غير متاحة مؤقتاً. يرجى المحاولة لاحقاً.";
      const retrySeconds = Number(response.headers.get("Retry-After")) || 60;
      clearTimeout(wisdomRefreshTimeout);
      wisdomRefreshTimeout = setTimeout(loadTodaysWisdom, Math.max(60, Math.min(retrySeconds, 3600)) * 1000);
      return;
    }
    if (!response.ok) {
      throw new Error(`Server responded with ${response.status}: ${response.statusText}`);
    }
    const data = await response.json();
    wisdomEl.textContent = data.message || data.body || "لا توجد حكمة متاحة";
    clearTimeout(wisdomRefreshTimeout);
    if (Number.isFinite(data.nextRotation)) {
      const delay = Math.max(1000, data.nextRotation * 1000 - Date.now() + 250);
      wisdomRefreshTimeout = setTimeout(loadTodaysWisdom, Math.min(delay, 2147483647));
    }
    
    console.log("✅ Hikmah loaded successfully");
  } catch (error) {
    console.error("❌ Failed to load hikmah:", error);
    wisdomEl.textContent = "فشل الاتصال بالخادم. يرجى التحقق من الاتصال بالإنترنت.";
    
    // Show detailed error in console for debugging
    if (API_URL.includes('YOUR_APP_NAME')) {
      console.error("⚠️ API_URL not configured! Update RENDER_BACKEND_URL in script.js");
    }
  }
}

// Fetch VAPID public key from server
async function getVAPIDPublicKey() {
  try {
    const response = await fetch(API_URL + "/api/vapid-public-key");
    if (!response.ok) {
      throw new Error(`Failed to fetch VAPID key: ${response.status}`);
    }
    const data = await response.json();
    if (!data.publicKey) {
      throw new Error("Server did not return a valid VAPID public key");
    }
    return data.publicKey;
  } catch (error) {
    console.error("❌ Failed to fetch VAPID key:", error);
    throw new Error("فشل الاتصال بالخادم. تأكد من تشغيل الخادم الخلفي.");
  }
}

// ====================================
// Service Worker Functions
// ====================================

// Register Service Worker
async function registerServiceWorker() {
  if (!("serviceWorker" in navigator)) {
    throw new Error("Service Workers not supported in this browser");
  }

  const registration = await navigator.serviceWorker.register("/sw.js");
  console.log("✅ Service Worker registered:", registration);
  
  // Wait for the service worker to be ready
  await navigator.serviceWorker.ready;
  return registration;
}

// ====================================
// Subscription Functions
// ====================================

// Subscribe to push notifications
async function subscribeToPush() {
  try {
    // Disable button during subscription
    subscribeBtn.disabled = true;
    btnText.textContent = "جاري الاشتراك...";
    updateStatus("جاري تفعيل الإشعارات...");

    // Check notification permission
    if (Notification.permission === "denied") {
      throw new Error("لقد قمت بحظر الإشعارات. يرجى السماح بها من إعدادات المتصفح.");
    }

    // Request notification permission
    const permission = await Notification.requestPermission();
    if (permission !== "granted") {
      throw new Error("يجب السماح بالإشعارات للاشتراك");
    }

    // Get selected frequency
    const frequency = getSelectedFrequency();
    console.log("📅 Selected frequency:", frequency, "hours");

    // Get VAPID public key
    if (!vapidPublicKey) {
      vapidPublicKey = await getVAPIDPublicKey();
      console.log("🔑 VAPID key received:", vapidPublicKey.substring(0, 20) + "...");
    }

    // Register service worker
    const registration = await registerServiceWorker();

    // Subscribe to push
    const subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKey),
    });

    console.log("📬 Push subscription created:", subscription);

    // Send subscription to server with frequency preference
    const response = await fetch(API_URL + "/api/subscribe", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        subscription: subscription,
        frequency: frequency
      }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error("Failed to save subscription: " + errorText);
    }

    const result = await response.json();
    console.log("✅ Subscription saved:", result);

    // Store subscription details in localStorage for countdown
    const subscriptionData = {
      frequency: frequency,
      subscribedAt: Date.now(),
      lastNotification: Date.now() // First notification sent immediately
    };
    localStorage.setItem('hikmahSubscription', JSON.stringify(subscriptionData));

    // Update UI
    btnText.textContent = "✓ مشترك";
    subscribeBtn.classList.add("subscribed");
    subscribeBtn.style.display = "none";
    unsubscribeBtn.style.display = "inline-block";

    
    const frequencyText = frequency === 1 ? "كل دقيقة (للتجربة)" 
      : frequency === 6 ? "كل 6 ساعات" 
      : frequency === 12 ? "كل 12 ساعة" 
      : "كل 24 ساعة";
    updateStatus(`تم الاشتراك! ستستقبل حكمة ${frequencyText} 🎉`);

    // Start personal countdown based on user's frequency
    startPersonalCountdown();

    // Hide frequency selection and instructions
    if (frequencySection) frequencySection.style.display = "none";
    if (infoSection) infoSection.style.display = "none";
    document.querySelector(".subtitle").style.display = "none";

  } catch (error) {
    console.error("❌ Subscription failed:", error);
    updateStatus(error.message || "فشل الاشتراك. حاول مرة أخرى.", true);
    
    // Re-enable button
    subscribeBtn.disabled = false;
    btnText.textContent = "اشترك الآن";
  }
}

// Check if already subscribed
async function checkSubscription() {
  try {
    if (!("serviceWorker" in navigator)) {
      return false;
    }

    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
      return false;
    }

    const subscription = await registration.pushManager.getSubscription();
    if (subscription) {
      // Already subscribed
      console.log("✅ Already subscribed");
      btnText.textContent = "✓ مشترك";
      subscribeBtn.classList.add("subscribed");
      subscribeBtn.style.display = "none";
      unsubscribeBtn.style.display = "inline-block";
      updateStatus("أنت مشترك بالفعل في الحكمة اليومية 🎉");
      
      // Hide UI elements
      if (frequencySection) frequencySection.style.display = "none";
      if (infoSection) infoSection.style.display = "none";
      document.querySelector(".subtitle").style.display = "none";
      
      // Start personal countdown if subscription data exists
      startPersonalCountdown();
      
      return true;
    }

    return false;
  } catch (error) {
    console.error("Error checking subscription:", error);
    return false;
  }
}

// Unsubscribe from push notifications
async function unsubscribeFromPush() {
  try {
    // Disable button during unsubscription
    unsubscribeBtn.disabled = true;
    unsubBtnText.textContent = "جاري الإلغاء...";
    updateStatus("جاري إلغاء الاشتراك...");

    const registration = await navigator.serviceWorker.getRegistration();
    if (!registration) {
      throw new Error("Service Worker not found");
    }

    const subscription = await registration.pushManager.getSubscription();
    if (!subscription) {
      throw new Error("No active subscription found");
    }

    // Unsubscribe from push
    const success = await subscription.unsubscribe();
    if (!success) {
      throw new Error("Failed to unsubscribe");
    }

    console.log("✅ Unsubscribed successfully");

    // Update UI
    unsubscribeBtn.style.display = "none";
    subscribeBtn.style.display = "inline-block";
    subscribeBtn.classList.remove("subscribed");
    subscribeBtn.disabled = false;
    btnText.textContent = "اشترك الآن";
    updateStatus("تم إلغاء الاشتراك بنجاح");

    // Clear localStorage
    localStorage.removeItem('hikmahSubscription');
    
    // Stop countdown
    if (countdownInterval) {
      clearInterval(countdownInterval);
      countdownInterval = null;
    }
    if (countdownEl) {
      countdownEl.textContent = '';
    }

    // Show frequency selection and instructions again
    if (frequencySection) frequencySection.style.display = "block";
    if (infoSection) infoSection.style.display = "block";
    document.querySelector(".subtitle").style.display = "block";

    // Re-enable unsubscribe button
    unsubscribeBtn.disabled = false;
    unsubBtnText.textContent = "إلغاء الاشتراك";

  } catch (error) {
    console.error("❌ Unsubscribe failed:", error);
    updateStatus(error.message || "فشل إلغاء الاشتراك. حاول مرة أخرى.", true);
    
    unsubscribeBtn.disabled = false;
    unsubBtnText.textContent = "إلغاء الاشتراك";
  }
}

// ====================================
// Event Listeners
// ====================================

subscribeBtn.addEventListener("click", subscribeToPush);
unsubscribeBtn.addEventListener("click", unsubscribeFromPush);

// ====================================
// Auto-Refresh Hikmah
// ====================================

// Personal countdown based on user's chosen frequency
function startPersonalCountdown() {
  // Get subscription data fromlocal storage
  const subData = localStorage.getItem('hikmahSubscription');
  if (!subData) {
    console.log('⚠️ No subscription data found for countdown');
    return;
  }

  const subscription = JSON.parse(subData);
  const { frequency, lastNotification } = subscription;

  // Clear existing interval
  if (countdownInterval) {
    clearInterval(countdownInterval);
  }

  function updateCountdown() {
    const now = Date.now();
    
    // Calculate next notification time based on frequency
    let frequencyMs;
    if (frequency === 1) {
      frequencyMs = 1 * 60 * 1000; // 1 minute
    } else {
      frequencyMs = frequency * 60 * 60 * 1000; // hours to milliseconds
    }
    
    const nextNotification = lastNotification + frequencyMs;
    const timeLeft = nextNotification - now;

    if (timeLeft <= 0) {
      // Time's up! Clear the current interval first
      clearInterval(countdownInterval);
      countdownInterval = null;
      
      // Update last notification time
      const updatedData = {
        ...subscription,
        lastNotification: now
      };
      localStorage.setItem('hikmahSubscription', JSON.stringify(updatedData));
      
      countdownEl.textContent = "🔔 حان موعد الإشعار!";
      
      // Reload hikmah
      loadTodaysWisdom();
      
      // Restart countdown with fresh data
      setTimeout(() => startPersonalCountdown(), 2000);
      return;
    }

    // Calculate hours, minutes, seconds
    const hours = Math.floor(timeLeft / (1000 * 60 * 60));
    const minutes = Math.floor((timeLeft % (1000 * 60 * 60)) / (1000 * 60));
    const seconds = Math.floor((timeLeft % (1000 * 60)) / 1000);

    // Format display based on frequency
    let displayText;
    if (frequency === 1) {
      // For 1 minute, show only seconds if less than 1 minute
      if (hours === 0 && minutes === 0) {
        displayText = `⏰ الإشعار التالي خلال: ${seconds} ثانية`;
      } else {
        const minutesStr = String(minutes).padStart(2, '0');
        const secondsStr = String(seconds).padStart(2, '0');
        displayText = `⏰ الإشعار التالي خلال: ${minutesStr}:${secondsStr}`;
      }
    } else {
      // For hours, show full HH:MM:SS
      const hoursStr = String(hours).padStart(2, '0');
      const minutesStr = String(minutes).padStart(2, '0');
      const secondsStr = String(seconds).padStart(2, '0');
      displayText = `⏰ الإشعار التالي خلال: ${hoursStr}:${minutesStr}:${secondsStr}`;
    }

    countdownEl.textContent = displayText;
  }

  // Update immediately
  updateCountdown();

  // Update every second
  countdownInterval = setInterval(updateCountdown, 1000);
}

// Auto-refresh hikmah periodically
function startAutoRefresh() {
  // Check every 10 seconds if it's time to refresh based on countdown
  setInterval(() => {
    const subData = localStorage.getItem('hikmahSubscription');
    if (subData) {
      const subscription = JSON.parse(subData);
      const { frequency, lastNotification } = subscription;
      
      let frequencyMs;
      if (frequency === 1) {
        frequencyMs = 1 * 60 * 1000; // 1 minute
      } else {
        frequencyMs = frequency * 60 * 60 * 1000;
      }
      
      const nextNotification = lastNotification + frequencyMs;
      
      if (Date.now() >= nextNotification) {
        console.log("🔄 Auto-refreshing hikmah based on frequency...");
        loadTodaysWisdom();
      }
    }
  }, 10000); // Check every 10 seconds
}

// ====================================
// Initialization
// ====================================

// Style the testing option (1-minute frequency)
function styleTestingOption() {
  const testingOption = document.getElementById('testingOption');
  const testingTitle = testingOption?.querySelector('.frequency-title');
  
  if (testingOption) {
    // Orange border and background for testing option
    testingOption.style.borderColor = '#ff9800';
    testingOption.style.background = 'rgba(255, 152, 0, 0.1)';
  }
  
  if (testingTitle) {
    // Orange text for the title
    testingTitle.style.color = '#ff9800';
  }
}

// Load wisdom and check subscription status on page load
document.addEventListener("DOMContentLoaded", () => {
  console.log("🚀 Daily Hikmah app initialized");
  
  // Apply styling to testing option
  styleTestingOption();
  
  loadTodaysWisdom();
  checkSubscription();
  startAutoRefresh();
});

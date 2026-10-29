// ====================================
// Service Worker for Daily Hikmah
// ====================================
// This runs in the background and can receive push notifications
// even when the browser is closed or the tab is not active

const CACHE_NAME = "daily-hikmah-v2";

// ====================================
// Install Event
// ====================================
self.addEventListener("install", (event) => {
  console.log("[Service Worker] Installing...");
  self.skipWaiting(); // Activate immediately
});

// ====================================
// Activate Event
// ====================================
self.addEventListener("activate", (event) => {
  console.log("[Service Worker] Activated");
  event.waitUntil(
    // Clean up old caches
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== CACHE_NAME) {
            console.log("[Service Worker] Deleting old cache:", cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  // Take control of all pages immediately
  event.waitUntil(self.clients.claim());
});

// ====================================
// Push Event (Most Important)
// ====================================
// This event fires when a push notification is received from the server
self.addEventListener("push", (event) => {
  console.log("[Service Worker] Push notification received");

  let notificationData = {
    title: "حكمة اليوم",
    message: "لديك حكمة جديدة!",
  };

  // Parse the push payload from the server
  if (event.data) {
    try {
      notificationData = event.data.json();
      console.log("[Service Worker] Notification data:", notificationData);
    } catch (error) {
      console.error("[Service Worker] Failed to parse push data:", error);
      // If parsing fails, try to read as text
      try {
        const textData = event.data.text();
        notificationData.message = textData;
      } catch (e) {
        console.error("[Service Worker] Could not read push data");
      }
    }
  }

  const options = {
    body: notificationData.message || notificationData.body,
    icon: "/icon-192.png", // Optional: add your app icon (192x192px)
    badge: "/badge-72.png", // Optional: add a badge icon (72x72px) 
    vibrate: [200, 100, 200], // Vibration pattern for mobile devices
    tag: "daily-hikmah", // Replaces previous notifications with same tag
    requireInteraction: false, // Auto-close after some time
    renotify: true, // Vibrate even if previous notification exists
    data: {
      url: self.registration.scope || "/", // URL to open when clicked
      timestamp: Date.now(),
    },
    lang: "ar", // Arabic language
    dir: "rtl", // Right-to-left direction for Arabic text
    silent: false, // Play notification sound
  };

  // Display the notification
  event.waitUntil(
    self.registration.showNotification(
      notificationData.title || "حكمة اليوم",
      options
    )
  );
});

// ====================================
// Notification Click Event
// ====================================
// This event fires when the user clicks on the notification
self.addEventListener("notificationclick", (event) => {
  console.log("[Service Worker] Notification clicked");
  
  // Close the notification
  event.notification.close();

  // Open or focus the app window
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true })
      .then((clientList) => {
        // If a window is already open, focus it
        for (const client of clientList) {
          if (client.url.includes(self.registration.scope) && "focus" in client) {
            console.log("[Service Worker] Focusing existing window");
            return client.focus();
          }
        }
        
        // Otherwise, open a new window
        if (self.clients.openWindow) {
          const urlToOpen = event.notification.data?.url || "/";
          console.log("[Service Worker] Opening new window:", urlToOpen);
          return self.clients.openWindow(urlToOpen);
        }
      })
  );
});

// ====================================
// Notification Close Event
// ====================================
self.addEventListener("notificationclose", (event) => {
  console.log("[Service Worker] Notification closed by user");
  // Optional: Send analytics or tracking data here
});

// ====================================
// Message Event
// ====================================
// Handle messages from the main app
self.addEventListener("message", (event) => {
  console.log("[Service Worker] Message received:", event.data);
  
  if (event.data && event.data.type === "SKIP_WAITING") {
    self.skipWaiting();
  }
});

// ====================================
// Push Subscription Change Event
// ====================================
// This fires when the push subscription expires or changes
self.addEventListener("pushsubscriptionchange", (event) => {
  console.log("[Service Worker] Push subscription changed");
  
  // Re-subscribe with the new subscription
  event.waitUntil(
    self.registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: event.oldSubscription?.options.applicationServerKey,
    })
    .then((newSubscription) => {
      console.log("[Service Worker] Re-subscribed:", newSubscription);
      // TODO: Send new subscription to backend
      // You might want to call your /subscribe endpoint here
    })
    .catch((error) => {
      console.error("[Service Worker] Re-subscription failed:", error);
    })
  );
});

console.log("[Service Worker] Loaded and ready for push notifications 🚀");

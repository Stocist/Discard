// Bump this whenever the caching strategy changes — `activate` purges any
// cache whose name differs, so a bump invalidates stale precaches.
const CACHE_NAME = 'discard-v2';

// Only content-hashed, immutable build assets are safe to serve cache-first.
// HTML and API responses must stay network-first or the app pins itself to a
// stale bundle across deploys.
const IMMUTABLE_PREFIX = '/_app/immutable/';

self.addEventListener('install', (event) => {
    // Activate the new SW immediately instead of waiting for all tabs to close.
    self.skipWaiting();
    event.waitUntil(caches.open(CACHE_NAME));
});

self.addEventListener('activate', (event) => {
    event.waitUntil(
        Promise.all([
            caches.keys().then((keys) =>
                Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
            ),
            self.clients.claim()
        ])
    );
});

// Cache a response only if it's a cacheable GET (the Cache API rejects
// non-GET requests, which is what spammed "method 'POST' is unsupported").
function putIfGet(request, response) {
    if (request.method !== 'GET' || !response || !response.ok) return;
    const clone = response.clone();
    caches.open(CACHE_NAME).then((cache) => cache.put(request, clone)).catch(() => {});
}

self.addEventListener('fetch', (event) => {
    const { request } = event;

    // Never intercept non-GET (POST/PUT/etc.) — let them go straight to the
    // network. This also avoids touching the WebSocket upgrade.
    if (request.method !== 'GET') return;

    const url = new URL(request.url);

    // Immutable hashed assets: cache-first (filename changes on every build,
    // so a cached entry is never stale).
    if (url.pathname.startsWith(IMMUTABLE_PREFIX)) {
        event.respondWith(
            caches.match(request).then(
                (cached) =>
                    cached ||
                    fetch(request).then((response) => {
                        putIfGet(request, response);
                        return response;
                    })
            )
        );
        return;
    }

    // Everything else (HTML navigations, /api/ GETs, manifest, icons):
    // network-first so a deploy is picked up immediately; fall back to cache
    // only when offline.
    event.respondWith(
        fetch(request)
            .then((response) => {
                putIfGet(request, response);
                return response;
            })
            .catch(() => caches.match(request))
    );
});

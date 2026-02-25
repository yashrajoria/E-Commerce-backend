package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"bff-service/clients"
	"bff-service/config"
	"bff-service/controllers"
	"bff-service/routes"

	"github.com/redis/go-redis/v9"

	"github.com/gin-gonic/gin"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	// ── CloudWatch Logs + Metrics ──
	var metricsClient *awspkg.MetricsClient
	if os.Getenv("CLOUDWATCH_ENABLED") == "true" {
		cwCtx := context.Background()
		cwLogs, err := awspkg.NewCloudWatchLogsClient(cwCtx, "bff-service")
		if err != nil {
			log.Printf("[BFF] CloudWatch Logs init failed: %v", err)
		} else {
			log.Println("[BFF] CloudWatch Logs enabled")
			_ = cwLogs // writes happen via logger; keep reference
		}
		mc, err := awspkg.NewMetricsClient(cwCtx)
		if err != nil {
			log.Printf("[BFF] CloudWatch Metrics init failed: %v", err)
		} else {
			metricsClient = mc
			log.Println("[BFF] CloudWatch Metrics enabled")
		}
	}

	timeout, err := time.ParseDuration(cfg.RequestTimeout)
	if err != nil {
		timeout = 10 * time.Second
	}

	gateway := clients.NewGatewayClient(cfg.APIGatewayURL, timeout)

	// Initialize Redis (optional - for idempotency)
	var redisClient *redis.Client
	if addr := os.Getenv("REDIS_URL"); addr != "" {
		opts, err := redis.ParseURL(addr)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}
		redisClient = redis.NewClient(opts)
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			log.Fatalf("failed to connect to Redis: %v", err)
		}
		log.Println("Connected to Redis (BFF)")
	}

	controller := controllers.NewBFFController(gateway, redisClient)

	r := gin.New()
	r.Use(gin.Recovery())

	// Initialize structured logger for request logging
	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync()

	// Structured HTTP request logging → CloudWatch
	r.Use(func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("body_size", c.Writer.Size()),
		}
		switch {
		case status >= 500:
			zapLogger.Error("http_request", fields...)
		case status >= 400:
			zapLogger.Warn("http_request", fields...)
		default:
			zapLogger.Info("http_request", fields...)
		}
	})

	// ── HTTP metrics middleware (inline) ──
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if metricsClient != nil {
			go func(path, method string, status int, dur time.Duration) {
				mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				dims := map[string]string{"Service": "bff-service", "Method": method, "Path": path}
				_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPRequests, dims)
				_ = metricsClient.RecordLatency(mctx, awspkg.MetricHTTPLatency, dur, dims)
				if status >= 500 {
					_ = metricsClient.RecordCount(mctx, awspkg.MetricHTTPErrors, dims)
				}
			}(c.Request.URL.Path, c.Request.Method, c.Writer.Status(), time.Since(start))
		}
	})

	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `<!doctype html>
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>E-Commerce API — Documentation</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=Berkeley+Mono&family=DM+Sans:wght@300;400;500;600&display=swap" rel="stylesheet" />
  <style>
    /* ── Reset & Base ─────────────────────────────────────────────────────── */
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    :root {
      --bg:          #0b0e14;
      --surface:     #111520;
      --surface-2:   #161b27;
      --border:      #1e2535;
      --border-2:    #252d42;
      --text:        #c9d1e0;
      --text-dim:    #5a6580;
      --text-muted:  #3a4258;
      --accent:      #3b82f6;
      --accent-glow: rgba(59,130,246,0.18);
      --accent-dim:  rgba(59,130,246,0.08);
      --green:       #22c55e;
      --amber:       #f59e0b;
      --red:         #ef4444;
      --mono:        'Berkeley Mono', 'Fira Code', 'Cascadia Code', monospace;
      --sans:        'DM Sans', system-ui, sans-serif;
      --header-h:    52px;
      --toolbar-h:   44px;
      --total-h:     96px;
    }

    html, body { height: 100%; overflow: hidden; background: var(--bg); color: var(--text); font-family: var(--sans); }

    /* ── Header ────────────────────────────────────────────────────────────── */
    #header {
      position: fixed; top: 0; left: 0; right: 0; z-index: 100;
      height: var(--header-h);
      background: var(--surface);
      border-bottom: 1px solid var(--border);
      display: flex; align-items: center; gap: 0;
      padding: 0;
    }

    .header-logo {
      display: flex; align-items: center; gap: 10px;
      padding: 0 20px;
      border-right: 1px solid var(--border);
      height: 100%;
      flex-shrink: 0;
    }
    .header-logo-icon {
      width: 28px; height: 28px;
      background: var(--accent);
      border-radius: 6px;
      display: flex; align-items: center; justify-content: center;
      font-size: 13px; font-weight: 700; color: white; font-family: var(--mono);
      letter-spacing: -0.5px;
      flex-shrink: 0;
    }
    .header-logo-text {
      display: flex; flex-direction: column; gap: 1px;
    }
    .header-logo-title {
      font-size: 13px; font-weight: 600; color: #e2e8f0; letter-spacing: 0.2px;
    }
    .header-logo-sub {
      font-size: 10px; color: var(--text-dim); letter-spacing: 0.5px; text-transform: uppercase; font-family: var(--mono);
    }

    .header-nav {
      display: flex; align-items: center; height: 100%; flex: 1; gap: 2px; padding: 0 12px;
    }

    .nav-btn {
      display: flex; align-items: center; gap: 6px;
      padding: 6px 12px; border-radius: 6px;
      font-size: 12.5px; font-weight: 500; color: var(--text-dim);
      cursor: pointer; border: none; background: none;
      transition: color 0.15s, background 0.15s;
      text-decoration: none; font-family: var(--sans);
      white-space: nowrap;
    }
    .nav-btn:hover { color: var(--text); background: var(--accent-dim); }
    .nav-btn.active { color: var(--accent); background: var(--accent-dim); }
    .nav-btn svg { opacity: 0.7; flex-shrink: 0; }
    .nav-btn:hover svg, .nav-btn.active svg { opacity: 1; }

    .header-right {
      display: flex; align-items: center; gap: 8px;
      padding: 0 16px;
      margin-left: auto;
      border-left: 1px solid var(--border);
      height: 100%;
    }

    .version-badge {
      font-family: var(--mono); font-size: 11px;
      background: var(--accent-dim); border: 1px solid rgba(59,130,246,0.25);
      color: var(--accent); padding: 2px 8px; border-radius: 4px;
      letter-spacing: 0.3px;
    }

    .icon-btn {
      width: 30px; height: 30px; border-radius: 6px;
      display: flex; align-items: center; justify-content: center;
      border: 1px solid var(--border); background: none; cursor: pointer;
      color: var(--text-dim); transition: all 0.15s;
    }
    .icon-btn:hover { color: var(--text); border-color: var(--border-2); background: var(--surface-2); }

    /* ── Toolbar ───────────────────────────────────────────────────────────── */
    #toolbar {
      position: fixed; top: var(--header-h); left: 0; right: 0; z-index: 99;
      height: var(--toolbar-h);
      background: var(--bg);
      border-bottom: 1px solid var(--border);
      display: flex; align-items: center; gap: 10px;
      padding: 0 16px;
    }

    .search-wrap {
      display: flex; align-items: center; gap: 8px;
      background: var(--surface); border: 1px solid var(--border);
      border-radius: 7px; padding: 0 10px;
      flex: 1; max-width: 380px;
      transition: border-color 0.15s;
    }
    .search-wrap:focus-within { border-color: var(--accent); }
    .search-wrap svg { color: var(--text-dim); flex-shrink: 0; }
    .search-wrap input {
      background: none; border: none; outline: none;
      font-size: 13px; color: var(--text); font-family: var(--sans);
      padding: 8px 0; width: 100%;
    }
    .search-wrap input::placeholder { color: var(--text-muted); }
    .kbd {
      font-family: var(--mono); font-size: 10px; color: var(--text-dim);
      border: 1px solid var(--border); border-radius: 3px;
      padding: 1px 5px; flex-shrink: 0;
    }

    .toolbar-divider {
      width: 1px; height: 20px; background: var(--border); flex-shrink: 0;
    }

    .filter-group {
      display: flex; align-items: center; gap: 4px;
    }
    .filter-label {
      font-size: 11px; color: var(--text-dim); font-family: var(--mono);
      text-transform: uppercase; letter-spacing: 0.5px; margin-right: 4px;
    }
    .filter-chip {
      font-size: 11.5px; padding: 3px 10px; border-radius: 5px;
      border: 1px solid var(--border); background: none;
      color: var(--text-dim); cursor: pointer; font-family: var(--sans);
      transition: all 0.15s; white-space: nowrap;
    }
    .filter-chip:hover { border-color: var(--border-2); color: var(--text); }
    .filter-chip.on { border-color: var(--accent); color: var(--accent); background: var(--accent-dim); }
    .filter-chip.admin.on { border-color: var(--amber); color: var(--amber); background: rgba(245,158,11,0.08); }
    .filter-chip.public.on { border-color: var(--green); color: var(--green); background: rgba(34,197,94,0.08); }

    .toolbar-spacer { flex: 1; }

    .spec-link {
      display: flex; align-items: center; gap: 6px;
      font-size: 12px; color: var(--text-dim); text-decoration: none;
      padding: 5px 10px; border-radius: 6px; border: 1px solid var(--border);
      font-family: var(--mono); transition: all 0.15s;
    }
    .spec-link:hover { color: var(--accent); border-color: var(--accent); background: var(--accent-dim); }

    /* ── Architecture Panel ────────────────────────────────────────────────── */
    #arch-panel {
      position: fixed; top: var(--total-h); left: 0; right: 0; bottom: 0; z-index: 90;
      background: var(--bg);
      display: none; flex-direction: column;
    }
    #arch-panel.open { display: flex; animation: fadeIn 0.2s ease; }

    .arch-inner {
      flex: 1; display: flex; flex-direction: column;
      padding: 20px 24px; gap: 16px; overflow: hidden;
    }
    .arch-header {
      display: flex; align-items: center; gap: 12px;
    }
    .arch-title {
      font-size: 13px; font-weight: 600; color: #e2e8f0;
      font-family: var(--mono);
    }
    .arch-subtitle {
      font-size: 12px; color: var(--text-dim);
    }
    .arch-actions {
      display: flex; gap: 8px; margin-left: auto;
    }
    .arch-frame-wrap {
      flex: 1; border-radius: 10px; overflow: hidden;
      border: 1px solid var(--border);
      background: #fff;
    }
    #arch-frame { width: 100%; height: 100%; border: none; display: block; }

    /* ── Redoc Container ───────────────────────────────────────────────────── */
    #docs-panel {
      position: fixed; top: var(--total-h); left: 0; right: 0; bottom: 0;
      overflow-y: auto; overflow-x: hidden;
    }

    #redoc-container {
      min-height: 100%;
    }

    /* ── Redoc Theme Overrides ─────────────────────────────────────────────── */
    /* sidebar */
    .menu-content { background: var(--surface) !important; border-right: 1px solid var(--border) !important; }
    .scrollbar-container { background: var(--surface) !important; }

    /* ── Status indicator ─────────────────────────────────────────────────── */
    .status-dot {
      width: 6px; height: 6px; border-radius: 50%;
      background: var(--green);
      box-shadow: 0 0 6px var(--green);
      flex-shrink: 0;
      animation: pulse 2.5s ease-in-out infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.4; }
    }
    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(-4px); }
      to   { opacity: 1; transform: translateY(0); }
    }

    /* ── Toast ────────────────────────────────────────────────────────────── */
    #toast {
      position: fixed; bottom: 20px; right: 20px; z-index: 999;
      background: var(--surface-2); border: 1px solid var(--border-2);
      color: var(--text); font-size: 13px; padding: 10px 16px;
      border-radius: 8px; box-shadow: 0 4px 20px rgba(0,0,0,0.5);
      pointer-events: none; opacity: 0;
      transition: opacity 0.2s;
      font-family: var(--sans);
      display: flex; align-items: center; gap: 8px;
    }
    #toast.show { opacity: 1; }

    /* ── Scrollbar ────────────────────────────────────────────────────────── */
    #docs-panel::-webkit-scrollbar { width: 6px; }
    #docs-panel::-webkit-scrollbar-track { background: var(--bg); }
    #docs-panel::-webkit-scrollbar-thumb { background: var(--border-2); border-radius: 3px; }
    #docs-panel::-webkit-scrollbar-thumb:hover { background: #3a4258; }

    /* ── Mobile ───────────────────────────────────────────────────────────── */
    @media (max-width: 680px) {
      .filter-group, .filter-label, .toolbar-divider { display: none; }
      .header-logo-sub { display: none; }
      .spec-link span { display: none; }
    }
  </style>
</head>
<body>

<!-- ── Header ──────────────────────────────────────────────────────────────── -->
<header id="header">
  <div class="header-logo">
    <div class="header-logo-icon">EC</div>
    <div class="header-logo-text">
      <div class="header-logo-title">E-Commerce API</div>
      <div class="header-logo-sub">Backend Documentation</div>
    </div>
  </div>

  <nav class="header-nav">
    <button class="nav-btn active" id="btn-docs" onclick="showDocs()">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
      Reference
    </button>
    <button class="nav-btn" id="btn-arch" onclick="toggleArch()">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="6" height="6" rx="1"/><rect x="16" y="3" width="6" height="6" rx="1"/><rect x="9" y="15" width="6" height="6" rx="1"/><path d="M5 9v3a1 1 0 001 1h12a1 1 0 001-1V9"/><line x1="12" y1="13" x2="12" y2="15"/></svg>
      Architecture
    </button>
  </nav>

  <div class="header-right">
    <div class="status-dot" title="Live spec"></div>
    <div class="version-badge" id="version-badge">v1.0.0</div>
    <a href="/docs/openapi.yaml" download title="Download spec" class="icon-btn">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
    </a>
    <button onclick="copySpecUrl()" title="Copy spec URL" class="icon-btn">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
    </button>
  </div>
</header>

<!-- ── Toolbar ─────────────────────────────────────────────────────────────── -->
<div id="toolbar">
  <div class="search-wrap">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
    <input type="search" id="search-input" placeholder="Search endpoints…" autocomplete="off" spellcheck="false" />
    <span class="kbd">/</span>
  </div>

  <div class="toolbar-divider"></div>

  <div class="filter-group">
    <span class="filter-label">Filter</span>
    <button class="filter-chip public" id="chip-public" onclick="toggleFilter('public')">Public</button>
    <button class="filter-chip admin" id="chip-admin" onclick="toggleFilter('admin')">🔐 Admin</button>
    <button class="filter-chip" id="chip-bff" onclick="toggleFilter('bff')">BFF</button>
    <button class="filter-chip" id="chip-internal" onclick="toggleFilter('internal')">Internal</button>
  </div>

  <div class="toolbar-spacer"></div>

  <a href="/docs/openapi.yaml" target="_blank" class="spec-link">
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
    <span>openapi.yaml</span>
  </a>
</div>

<!-- ── Architecture Panel ──────────────────────────────────────────────────── -->
<div id="arch-panel">
  <div class="arch-inner">
    <div class="arch-header">
      <div>
        <div class="arch-title">System Architecture</div>
        <div class="arch-subtitle">Microservice topology and request flow</div>
      </div>
      <div class="arch-actions">
        <a href="/docs/architecture.svg" target="_blank" class="spec-link">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
          <span>Open in new tab</span>
        </a>
        <button class="icon-btn" onclick="showDocs()" title="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
      </div>
    </div>
    <div class="arch-frame-wrap">
      <iframe id="arch-frame" src="/docs/architecture.svg"></iframe>
    </div>
  </div>
</div>

<!-- ── Docs Panel ──────────────────────────────────────────────────────────── -->
<div id="docs-panel">
  <div id="redoc-container"></div>
</div>

<!-- ── Toast ──────────────────────────────────────────────────────────────── -->
<div id="toast">
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
  <span id="toast-msg">Copied!</span>
</div>

<script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
<script>
  /* ── State ──────────────────────────────────────────────────────────────── */
  let activeFilters = new Set();
  let archOpen = false;

  /* ── Redoc init ─────────────────────────────────────────────────────────── */
  Redoc.init('/docs/openapi.yaml', {
    nativeScrollbars: true,
    scrollYOffset: 96,
    hideDownloadButton: true,
    disableSearch: true, // we use our own search
    theme: {
      colors: {
        primary:    { main: '#3b82f6' },
        success:    { main: '#22c55e' },
        warning:    { main: '#f59e0b' },
        error:      { main: '#ef4444' },
        text:       { primary: '#c9d1e0', secondary: '#5a6580' },
        border:     { dark: '#1e2535', light: '#1e2535' },
        responses:  { success: { color: '#22c55e', backgroundColor: 'rgba(34,197,94,0.06)', tabTextColor: '#22c55e' },
                      error:   { color: '#ef4444', backgroundColor: 'rgba(239,68,68,0.06)',  tabTextColor: '#ef4444' },
                      redirect:{ color: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.06)', tabTextColor: '#f59e0b' },
                      info:    { color: '#3b82f6', backgroundColor: 'rgba(59,130,246,0.06)', tabTextColor: '#3b82f6' } },
        http:       {
          get:     '#22c55e', post:   '#3b82f6', put:    '#f59e0b',
          delete:  '#ef4444', patch:  '#a855f7', head:   '#5a6580',
          options: '#5a6580'
        }
      },
      sidebar: {
        width: '280px',
        backgroundColor: '#111520',
        textColor: '#c9d1e0',
      },
      rightPanel: {
        backgroundColor: '#0d1117',
        width: '40%',
      },
      typography: {
        fontSize: '14px',
        lineHeight: '1.6',
        fontFamily: "'DM Sans', system-ui, sans-serif",
        headings: { fontFamily: "'DM Sans', system-ui, sans-serif", fontWeight: '600' },
        code: { fontSize: '13px', fontFamily: "'Berkeley Mono', 'Fira Code', monospace", backgroundColor: '#161b27', color: '#c9d1e0' },
        links: { color: '#3b82f6', visited: '#3b82f6', hover: '#60a5fa' }
      },
      schema: {
        requirePropertiesColor: '#ef4444',
        defaultDetailsWidth: '75%',
        typeNameColor: '#a78bfa',
        typeTitleColor: '#c9d1e0',
        requireLabelColor: '#ef4444',
        labelsTextSize: '0.85em',
        nestingSpacing: '1em',
        nestedBackground: '#161b27',
        arrow: { size: '1.1em', color: '#3a4258' },
      },
      codeBlock: { backgroundColor: '#0d1117' },
      logo: { maxHeight: '40px' }
    }
  }, document.getElementById('redoc-container'));

  /* ── Fetch version ──────────────────────────────────────────────────────── */
  fetch('/docs/openapi.yaml')
    .then(r => r.text())
    .then(txt => {
      const m = txt.match(/^\s*version:\s*['"]?([^\s'"]+)['"]?/m);
      if (m) document.getElementById('version-badge').textContent = 'v' + m[1].trim();
    })
    .catch(() => {});

  /* ── Navigation ─────────────────────────────────────────────────────────── */
  function showDocs() {
    archOpen = false;
    document.getElementById('arch-panel').classList.remove('open');
    document.getElementById('docs-panel').style.display = 'block';
    document.getElementById('btn-docs').classList.add('active');
    document.getElementById('btn-arch').classList.remove('active');
  }

  function toggleArch() {
    archOpen = !archOpen;
    document.getElementById('arch-panel').classList.toggle('open', archOpen);
    document.getElementById('docs-panel').style.display = archOpen ? 'none' : 'block';
    document.getElementById('btn-arch').classList.toggle('active', archOpen);
    document.getElementById('btn-docs').classList.toggle('active', !archOpen);
  }

  /* ── Search ─────────────────────────────────────────────────────────────── */
  document.getElementById('search-input').addEventListener('input', function () {
    // Proxy into Redoc's internal search box if it exists
    const redocSearch = document.querySelector('input[type=search]');
    if (redocSearch && redocSearch !== this) {
      redocSearch.value = this.value;
      redocSearch.dispatchEvent(new Event('input', { bubbles: true }));
    }
  });

  window.addEventListener('keydown', function (e) {
    if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey &&
        document.activeElement.tagName !== 'INPUT') {
      e.preventDefault();
      document.getElementById('search-input').focus();
    }
    if (e.key === 'Escape') {
      const si = document.getElementById('search-input');
      if (document.activeElement === si) { si.blur(); si.value = ''; }
    }
  });

  /* ── Filter chips (visual-only — Redoc sidebar filtering) ──────────────── */
  function toggleFilter(name) {
    const chip = document.getElementById('chip-' + name);
    if (activeFilters.has(name)) {
      activeFilters.delete(name);
      chip.classList.remove('on');
    } else {
      activeFilters.add(name);
      chip.classList.add('on');
    }
    applyFilters();
  }

  function applyFilters() {
    if (activeFilters.size === 0) {
      // Reset: show all sidebar items
      document.querySelectorAll('[data-item-id]').forEach(el => el.style.display = '');
      return;
    }

    // Map filter names to tag/path patterns
    const matchers = {
      public:   el => !el.querySelector('.http-verb') || el.textContent.includes('GET'),
      admin:    el => el.textContent.includes('Admin') || el.textContent.includes('🔐'),
      bff:      el => el.querySelector('a')?.getAttribute('href')?.includes('/bff/'),
      internal: el => el.textContent.toLowerCase().includes('internal'),
    };

    // Attempt to filter Redoc sidebar menu items
    const items = document.querySelectorAll('li[data-item-id], .sc-iJCRrI');
    items.forEach(el => {
      const show = [...activeFilters].some(f => matchers[f]?.(el));
      el.style.display = show ? '' : 'none';
    });
  }

  /* ── Copy spec URL ──────────────────────────────────────────────────────── */
  function copySpecUrl() {
    const url = window.location.origin + '/docs/openapi.yaml';
    navigator.clipboard.writeText(url).then(() => toast('Spec URL copied!')).catch(() => toast('Copy failed'));
  }

  /* ── Toast ──────────────────────────────────────────────────────────────── */
  let toastTimer;
  function toast(msg) {
    const el = document.getElementById('toast');
    document.getElementById('toast-msg').textContent = msg;
    el.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove('show'), 2200);
  }
</script>
</body>
</html>`)
	})
	r.GET("/docs/openapi.yaml", func(c *gin.Context) {
		c.File("/docs/openapi.yaml")
	})

	routes.RegisterRoutes(r, controller)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("[BFF] listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[BFF] server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[BFF] shutdown error: %v", err)
	}
}

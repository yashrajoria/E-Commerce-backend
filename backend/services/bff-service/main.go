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
<html>
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		<title>API Documentation</title>
		<style>
			body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial; }
			.topbar { background: #0d6efd; color: white; padding: 12px 20px; display:flex; align-items:center; gap:12px }
			.topbar h1 { font-size:16px; margin:0; }
			#redoc { height: calc(100vh - 52px); }
			@media (max-width: 600px) { .topbar h1 { font-size:14px } }
		</style>
	</head>
	<body>
		<div class="topbar">
			<h1>E-Commerce Backend API Docs</h1>
			<div style="opacity:0.9;font-size:13px">Interactive docs (read-only)</div>
		</div>
		<div style="padding:10px 20px; background:#f8f9fa; display:flex; gap:8px; align-items:center">
			<a id="view-arch" href="#" style="text-decoration:none;color:#0d6efd;font-weight:600">View Architecture</a>
			<a href="/docs/architecture.svg" target="_blank" style="text-decoration:none;color:#0d6efd">Open Architecture (new tab)</a>
			<span style="margin-left:auto;opacity:0.8;font-size:13px">Spec: <a href="/docs/openapi.yaml" style="color:#0d6efd">openapi.yaml</a></span>
		</div>
		<div id="redoc"></div>
		<div id="arch-container" style="display:none;padding:12px 20px;background:#fff;border-top:1px solid #e6e6e6">
			<iframe id="arch-frame" src="/docs/architecture.svg" style="width:100%;height:600px;border:0"></iframe>
		</div>
		<script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
		<script>
			// Initialize Redoc with a few UX-friendly options
			Redoc.init('/docs/openapi.yaml', {
				nativeScrollbars: true,
				scrollYOffset: 52,
				hideDownloadButton: false,
				theme: {
					colors: { primary: { main: '#0d6efd' } },
					typography: { fontSize: '14px' }
				}
			}, document.getElementById('redoc'));
			// Fetch version from YAML and show it in the header
			fetch('/docs/openapi.yaml').then(r => r.text()).then(txt => {
				const m = txt.match(/^\s*version:\s*(.+)$/m);
				if (m && m[1]) {
					const ver = m[1].trim();
					const el = document.createElement('div');
					el.style.opacity = '0.9';
					el.style.fontSize = '13px';
					el.style.marginLeft = 'auto';
					el.textContent = 'spec v' + ver;
					document.querySelector('.topbar').appendChild(el);
				}
			}).catch(()=>{});

			// Add download & open buttons
			(function addControls(){
				const container = document.createElement('div');
				container.style.marginLeft = '12px';
				container.style.display = 'flex';
				container.style.gap = '8px';

				const dl = document.createElement('a');
				dl.href = '/docs/openapi.yaml';
				dl.textContent = 'Download OpenAPI';
				dl.style.color = 'white';
				dl.style.textDecoration = 'none';
				dl.style.fontSize = '13px';

				const open = document.createElement('a');
				open.href = '/docs/openapi.yaml';
				open.target = '_blank';
				open.textContent = 'Open spec';
				open.style.color = 'white';
				open.style.textDecoration = 'none';
				open.style.fontSize = '13px';

				container.appendChild(dl);
				container.appendChild(open);
				document.querySelector('.topbar').appendChild(container);
			})();

			// Keyboard shortcut '/' to focus the search box
			window.addEventListener('keydown', function(e){
				if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey) {
					e.preventDefault();
					const input = document.querySelector('input[type=search]');
					if (input) input.focus();
				}
			});

		            // Toggle architecture viewer
		            document.getElementById('view-arch').addEventListener('click', function(e){
		                e.preventDefault();
		                const c = document.getElementById('arch-container');
		                if (c.style.display === 'none') c.style.display = 'block'; else c.style.display = 'none';
		            });
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

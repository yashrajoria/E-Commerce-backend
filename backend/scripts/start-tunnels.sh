#!/bin/bash
echo "Starting ngrok tunnels for all ShopSwift services..."

ngrok http 8080 --log=stdout > /tmp/ngrok-8080.log &
ngrok http 8081 --log=stdout > /tmp/ngrok-8081.log &
ngrok http 8082 --log=stdout > /tmp/ngrok-8082.log &
ngrok http 8083 --log=stdout > /tmp/ngrok-8083.log &
ngrok http 8084 --log=stdout > /tmp/ngrok-8084.log &
ngrok http 8085 --log=stdout > /tmp/ngrok-8085.log &
ngrok http 8086 --log=stdout > /tmp/ngrok-8086.log &
ngrok http 8087 --log=stdout > /tmp/ngrok-8087.log &
ngrok http 8088 --log=stdout > /tmp/ngrok-8088.log &
ngrok http 8089 --log=stdout > /tmp/ngrok-8089.log &
ngrok http 8090 --log=stdout > /tmp/ngrok-8090.log &
ngrok http 8091 --log=stdout > /tmp/ngrok-8091.log &
ngrok http 8092 --log=stdout > /tmp/ngrok-8092.log &
ngrok http 8099 --log=stdout > /tmp/ngrok-8099.log &

echo "Waiting for tunnels to establish..."
sleep 8

echo ""
echo "========================================="
echo "         ShopSwift ngrok URLs"
echo "========================================="
declare -A labels
labels[8080]="API Gateway      "
labels[8081]="Auth Service     "
labels[8082]="Product Service  "
labels[8083]="Order Service    "
labels[8084]="Inventory Service"
labels[8085]="User Service     "
labels[8086]="Cart Service     "
labels[8087]="Payment Service  "
labels[8088]="BFF Service      "
labels[8089]="Agent Service    "
labels[8090]="Promotion Service"
labels[8091]="Shipping Service "
labels[8092]="Notification Svc "
labels[8099]="Swagger Docs     "

for port in 8080 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092 8099; do
  url=$(grep -o 'url=https://[^ ]*' /tmp/ngrok-$port.log | head -1 | cut -d= -f2)
  if [ -z "$url" ]; then
    url="(tunnel not ready — use local IP instead)"
  fi
  echo "${labels[$port]} :$port → $url"
done
echo "========================================="
echo ""
echo "Or use direct local IP (same WiFi, faster):"
echo "  http://172.16.14.242:8080  ← API Gateway"
echo "  http://172.16.14.242:8088  ← BFF Service"
echo "  http://172.16.14.242:8099  ← Swagger Docs"
echo "========================================="
echo "Press Ctrl+C to stop all tunnels"
wait
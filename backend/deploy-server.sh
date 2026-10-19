#!/bin/bash

APP_DIR="/www/wwwroot/mini-agent/backend"
cd "$APP_DIR" || exit 1
chmod +x server

# 1. 初始化 .env（首次部署时使用 openssl 安全生成随机密钥）
if [ ! -f ".env" ]; then
  cp .env_rename_me .env
  KEY1=$(openssl rand -hex 32 2>/dev/null || echo "secret_access_key_mini_agent_123")
  KEY2=$(openssl rand -hex 32 2>/dev/null || echo "secret_refresh_key_mini_agent_456")
  sed -i "s/ACCESS_SECRET=/ACCESS_SECRET=${KEY1}/g" .env
  sed -i "s/REFRESH_SECRET=/REFRESH_SECRET=${KEY2}/g" .env
  sed -i "s/SSL=TRUE/SSL=FALSE/g" .env
  echo ">>> 首次部署：已自动初始化 .env 文件"
fi

# 2. 如果存在 Supervisor，优先使用 Supervisor
if command -v supervisorctl &> /dev/null && supervisorctl status mini-agent &> /dev/null; then
  supervisorctl restart mini-agent
  echo ">>> 已通过 Supervisor 重启服务"
  exit 0
fi

# 3. 使用 Linux 标准 systemd 进行系统级服务托管（永不挂起 SSH，支持开机自启与崩溃重启）
SERVICE_FILE="/etc/systemd/system/mini-agent.service"
if [ ! -f "$SERVICE_FILE" ]; then
  cat << 'EOF' > "$SERVICE_FILE"
[Unit]
Description=Mini-Agent Go Backend
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/www/wwwroot/mini-agent/backend
ExecStart=/www/wwwroot/mini-agent/backend/server
Restart=always
RestartSec=3
StandardOutput=append:/www/wwwroot/mini-agent/backend/app.log
StandardError=append:/www/wwwroot/mini-agent/backend/app.log

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload 2>/dev/null || true
  systemctl enable mini-agent 2>/dev/null || true
  echo ">>> 已自动注册 systemd 守护服务"
fi

systemctl daemon-reload 2>/dev/null || true
systemctl restart mini-agent 2>/dev/null || true

if systemctl is-active --quiet mini-agent; then
  echo ">>> Mini-Agent 后端已通过 systemd 成功启动并守护运行！"
  exit 0
fi

# 4. 兜底方案（若环境不支持 systemd）
pkill -f "./server" || true
nohup ./server > app.log 2>&1 < /dev/null &
sleep 1
echo ">>> Mini-Agent 后端已以后台守护模式启动！"
exit 0

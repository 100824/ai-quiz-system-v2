module.exports = {
  apps: [{
    name: 'ai-quiz-system',
    // 运行前先执行：npm run backend:build
    // 该命令会在 backend-go/ 目录下生成名为 server 的可执行文件
    script: 'backend-go/server',
    cwd: __dirname,
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '200M',
    env: {
      PORT: 8080
    },
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    error_file: './logs/pm2-error.log',
    out_file: './logs/pm2-out.log',
    merge_logs: true,
    min_uptime: '10s',
    max_restarts: 10,
    restart_delay: 5000
  }]
};

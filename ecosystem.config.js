module.exports = {
  apps: [{
    name: 'ai-quiz-system',
    script: 'backend/server.js',
    cwd: '/root/.openclaw/workspace/ai-quiz-system-v2',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '200M',
    env: {
      NODE_ENV: 'production',
      PORT: 8080
    },
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    error_file: './logs/pm2-error.log',
    out_file: './logs/pm2-out.log',
    merge_logs: true,
    // 异常重启配置
    min_uptime: '10s',
    max_restarts: 10,
    restart_delay: 5000
  }]
};

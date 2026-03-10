module.exports = {
  apps: [
    {
      name: 'go-epp-proxy',
      script: './epp-http-bridge',
      cwd: __dirname,
      instances: 1,
      autorestart: true,
      max_memory_restart: '512M',
      merge_logs: true,
      out_file: './logs/pm2-out.log',
      error_file: './logs/pm2-err.log',
      env: {
        EPP_ENV_FILE: './.env',
        EPP_LOG_FORMAT: 'json'
      }
    }
  ]
};

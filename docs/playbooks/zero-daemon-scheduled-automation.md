# Zero-Daemon Scheduled Automation Guide

Panduan integrasi dan otomasi terjadwal untuk SqlFox Job Runner (`sqlfox job run`) menggunakan penjadwal bawaan sistem operasi (Linux cron, systemd timer, macOS launchd, dan Windows Task Scheduler) tanpa memerlukan proses daemon background tambahan.

---

## 1. Anatomi File Job (`sqlfox-job.json`)

SqlFox menggunakan format JSON deklaratif murni yang mendukung substitusi variabel lingkungan (`$VAR` atau `${VAR}`).

```json
{
  "name": "Nightly Production Backup & Transfer",
  "tasks": [
    {
      "name": "Target Health Check",
      "type": "health-check",
      "target": "mysql://${DB_USER}:${DB_PASS}@127.0.0.1:3306/production",
      "timeout": "5s"
    },
    {
      "name": "Daily SQL Dump",
      "type": "dump",
      "target": "mysql://${DB_USER}:${DB_PASS}@127.0.0.1:3306/production",
      "output": "/var/backups/sqlfox/prod-backup-{DATE}.sql",
      "rotation": {
        "keep_last": 7,
        "max_age": "14d"
      }
    },
    {
      "name": "Mirror to Staging",
      "type": "transfer",
      "from": "mysql://${DB_USER}:${DB_PASS}@127.0.0.1:3306/production",
      "to": "sqlite:///var/data/staging-copy.db",
      "all_tables": true,
      "create_missing": true,
      "truncate": true
    }
  ]
}
```

### Pre-Flight Check (Dry-Run)
Sebelum menjadwalkan job di cron, lakukan verifikasi koneksi dan izin tulis:
```bash
sqlfox job check --file /etc/sqlfox/jobs/nightly.json
```

---

## 2. Template Penjadwalan Berbasis OS

### A. Linux Cron (crontab)
Jalankan setiap hari pukul 02:00 pagi:
```cron
# /etc/cron.d/sqlfox-nightly atau crontab -e
0 2 * * * DB_USER=root DB_PASS=secret /usr/local/bin/sqlfox job run --file /etc/sqlfox/jobs/nightly.json >> /var/log/sqlfox/nightly.log 2>&1
```

### B. Linux Systemd Timer (Rekomendasi untuk Server Modern)
Kelebihan: logging terpusat via `journalctl`, retry terstruktur, monitoring status.

1. **Service Unit (`/etc/systemd/system/sqlfox-nightly.service`)**:
```ini
[Unit]
Description=SqlFox Scheduled Maintenance Job
After=network.target

[Service]
Type=oneshot
User=sqlfox
Environment="DB_USER=app"
EnvironmentFile=/etc/sqlfox/env.secrets
ExecStart=/usr/local/bin/sqlfox job run --file /etc/sqlfox/jobs/nightly.json
StandardOutput=journal
StandardError=journal
```

2. **Timer Unit (`/etc/systemd/system/sqlfox-nightly.timer`)**:
```ini
[Unit]
Description=Run SqlFox Nightly Job at 02:00 UTC

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

Aktifkan:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sqlfox-nightly.timer
```

### C. macOS Launchd Plist
Simpan di `~/Library/LaunchAgents/com.sqlfox.nightly.plist`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sqlfox.nightly</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/sqlfox</string>
        <string>job</string>
        <string>run</string>
        <string>--file</string>
        <string>/Users/shared/sqlfox/nightly.json</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>2</integer>
        <key>Minute</key>
        <integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>/tmp/sqlfox-job.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/sqlfox-job-err.log</string>
</dict>
</plist>
```

Aktifkan:
```bash
launchctl load ~/Library/LaunchAgents/com.sqlfox.nightly.plist
```

### D. Windows Task Scheduler
Gunakan Command Prompt atau PowerShell Administrator:
```powershell
$action = New-ScheduledTaskAction -Execute "C:\sqlfox\sqlfox.exe" -Argument "job run --file C:\sqlfox\jobs\nightly.json"
$trigger = New-ScheduledTaskTrigger -Daily -At 2:00AM
Register-ScheduledTask -TaskName "SqlFox-Nightly-Job" -Action $action -Trigger $trigger -Description "SqlFox Automated Scheduled Maintenance"
```

---

## 3. Exit Code & Monitoring
- **`0`**: Semua tugas selesai dengan sukses.
- **`1`**: Kegagalan koneksi, I/O disk, atau syntax query.
- **`2`**: Terhambat oleh AST Safety Guard (destructive query tanpa `--force`).

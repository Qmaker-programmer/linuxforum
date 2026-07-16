// Copyright (C) 2026 Qmaker <andresavalosgallegos@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	backupsDir                 = "db/backups"
	defaultBackupIntervalHours = 120 // 5 days
)

func ensureBackupsDir() error {
	return os.MkdirAll(backupsDir, 0755)
}

// performBackup writes a consistent snapshot of the live database to
// backupsDir using SQLite's VACUUM INTO, which is safe to run against a
// database that's being read from and written to concurrently (unlike
// just copying the .db file, which under WAL mode can miss data that's
// still sitting in the -wal file).
func performBackup() {
	if err := ensureBackupsDir(); err != nil {
		fmt.Println("Error al crear el directorio de backups:", err)
		return
	}

	now := time.Now()
	backupPath := filepath.Join(backupsDir, fmt.Sprintf("forum-%s.db", now.Format("20060102-150405")))

	if _, err := db.Exec("VACUUM INTO ?", backupPath); err != nil {
		fmt.Println("Error al hacer backup de la base de datos:", err)
		return
	}

	fmt.Println("Backup de la base de datos creado:", backupPath, "-", now.Format("2006-01-02 15:04:05"))
}

// runPeriodicBackups waits backup_interval_hours (config.json) between
// backups, starting the count from process startup. It doesn't persist
// the schedule across restarts, same as every other periodic cleanup
// goroutine in this codebase.
func runPeriodicBackups() {
	for {
		time.Sleep(time.Duration(config.BackupIntervalHours) * time.Hour)
		performBackup()
	}
}

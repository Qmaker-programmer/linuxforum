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
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		slog.Error("No se pudo crear el directorio de backups", "err", err)
		return
	}

	now := time.Now()
	backupPath := filepath.Join(backupsDir, fmt.Sprintf("forum-%s.db", now.Format("20060102-150405")))

	if _, err := db.Exec("VACUUM INTO ?", backupPath); err != nil {
		slog.Error("No se pudo hacer el backup de la base de datos", "err", err)
		return
	}

	slog.Info("Backup de la base de datos creado", "path", backupPath)

	pruneOldBackups()
}

// pruneOldBackups deletes the oldest backup files once there are more
// than max_backups (config.json). 0 (the default) means no pruning —
// backups accumulate forever until removed by hand. Backup filenames are
// zero-padded timestamps (forum-YYYYMMDD-HHMMSS.db), so a plain
// lexicographic sort is also a chronological sort.
func pruneOldBackups() {
	if config.MaxBackups <= 0 {
		return
	}

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "forum-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= config.MaxBackups {
		return
	}

	sort.Strings(names)
	for _, name := range names[:len(names)-config.MaxBackups] {
		path := filepath.Join(backupsDir, name)
		if err := os.Remove(path); err != nil {
			slog.Error("No se pudo eliminar un backup viejo", "path", path, "err", err)
			continue
		}
		slog.Info("Backup viejo eliminado", "path", path)
	}
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

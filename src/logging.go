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
	"log/slog"
	"os"
	"strings"
)

// logLevel backs the default logger's handler. It's a *slog.LevelVar
// (not a plain slog.Level) so the verbosity can be raised or lowered at
// runtime after config.json is parsed, without having to rebuild the
// handler — initLogger() runs before loadConfig() can know log_level.
var logLevel = new(slog.LevelVar)

// initLogger installs the default slog logger. Call this before anything
// else in main() logs, so even config-loading failures go through it.
func initLogger() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(handler))
}

// applyLogLevel sets the logger's verbosity from config.json's
// log_level ("debug", "info", "warn", "error"). Unrecognized or empty
// values fall back to info.
func applyLogLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "warn", "warning":
		logLevel.Set(slog.LevelWarn)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}
}

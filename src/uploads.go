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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	maxImageUploadSize = 5 << 20 // 5 MB
	uploadsDir         = "web/uploads"
)

var allowedImageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func ensureUploadsDir() error {
	return os.MkdirAll(uploadsDir, 0755)
}

// saveUploadedImage reads the file uploaded under the given form field,
// validates its size and real content type (never trusting the filename
// or the client-supplied Content-Type), and stores it under uploadsDir
// with a random name. It returns the public URL to embed in Markdown.
func saveUploadedImage(r *http.Request, field string) (string, error) {
	file, header, err := r.FormFile(field)
	if err != nil {
		return "", fmt.Errorf("Selecciona una imagen para insertar.")
	}
	defer file.Close()

	if header.Size > maxImageUploadSize {
		return "", fmt.Errorf("La imagen supera el tamaño máximo permitido (5 MB).")
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("Error al leer la imagen.")
	}
	head = head[:n]

	contentType := http.DetectContentType(head)
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return "", fmt.Errorf("Formato de imagen no soportado. Usa PNG, JPEG, GIF o WEBP.")
	}

	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return "", fmt.Errorf("Error al generar el nombre del archivo.")
	}
	filename := hex.EncodeToString(name) + ext
	dest, err := os.Create(filepath.Join(uploadsDir, filename))
	if err != nil {
		return "", fmt.Errorf("Error al guardar la imagen.")
	}
	defer dest.Close()

	if _, err := dest.Write(head); err != nil {
		return "", fmt.Errorf("Error al guardar la imagen.")
	}
	if _, err := io.Copy(dest, file); err != nil {
		return "", fmt.Errorf("Error al guardar la imagen.")
	}

	return "/web/uploads/" + filename, nil
}

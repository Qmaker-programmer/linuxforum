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
	"bytes"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func TestExtractUploadedImagePaths(t *testing.T) {
	message := "texto ![a](/web/uploads/0123456789abcdef0123456789abcdef.png) y otra ![b](/web/uploads/fedcba9876543210fedcba9876543210.webp) más texto"
	got := extractUploadedImagePaths(message)
	if len(got) != 2 {
		t.Fatalf("expected 2 image paths, got %d: %v", len(got), got)
	}
	if got[0] != "/web/uploads/0123456789abcdef0123456789abcdef.png" {
		t.Errorf("unexpected first path: %s", got[0])
	}
	if got[1] != "/web/uploads/fedcba9876543210fedcba9876543210.webp" {
		t.Errorf("unexpected second path: %s", got[1])
	}
}

func TestDiffRemovedImages(t *testing.T) {
	oldMsg := "![a](/web/uploads/0123456789abcdef0123456789abcdef.png) ![b](/web/uploads/fedcba9876543210fedcba9876543210.webp)"
	newMsg := "![a](/web/uploads/0123456789abcdef0123456789abcdef.png)"

	removed := diffRemovedImages(oldMsg, newMsg)
	if len(removed) != 1 || removed[0] != "/web/uploads/fedcba9876543210fedcba9876543210.webp" {
		t.Errorf("expected only the webp image to be removed, got %v", removed)
	}
}

func newMultipartImageRequest(t *testing.T, fieldName, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/post", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestSaveUploadedImageAcceptsPNG(t *testing.T) {
	if err := ensureUploadsDir(); err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatal(err)
	}

	req := newMultipartImageRequest(t, "image", "test.png", png)
	if err := req.ParseMultipartForm(2 << 20); err != nil {
		t.Fatal(err)
	}

	url, err := saveUploadedImage(req, "image")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	t.Cleanup(func() { os.Remove(url[1:]) })

	if _, err := os.Stat(url[1:]); err != nil {
		t.Errorf("expected uploaded file to exist at %s: %v", url, err)
	}
}

func TestSaveUploadedImageRejectsNonImage(t *testing.T) {
	if err := ensureUploadsDir(); err != nil {
		t.Fatal(err)
	}

	req := newMultipartImageRequest(t, "image", "fake.png", []byte("esto no es una imagen"))
	if err := req.ParseMultipartForm(2 << 20); err != nil {
		t.Fatal(err)
	}

	before, _ := os.ReadDir(uploadsDir)

	if _, err := saveUploadedImage(req, "image"); err == nil {
		t.Error("expected an error for a non-image file")
	}

	after, _ := os.ReadDir(uploadsDir)
	if len(after) != len(before) {
		t.Error("expected no file to be created for a rejected upload")
	}
}

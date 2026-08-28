package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsImageAttachment(t *testing.T) {
	tests := []struct {
		name       string
		attachment attachment
		want       bool
	}{
		{name: "content type", attachment: attachment{Filename: "asset.bin", ContentType: "image/webp"}, want: true},
		{name: "jpg extension", attachment: attachment{Filename: "Photo.JPEG"}, want: true},
		{name: "video", attachment: attachment{Filename: "clip.mp4", ContentType: "video/mp4"}, want: false},
		{name: "missing metadata", attachment: attachment{Filename: "README"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isImageAttachment(tt.attachment); got != tt.want {
				t.Fatalf("isImageAttachment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafeFilename(t *testing.T) {
	tests := map[string]string{
		"normal file.png":      "normal file.png",
		"../../escape.png":     "escape.png",
		"windows\\escape.jpg":  "windows_escape.jpg",
		"control\x00name.webp": "control_name.webp",
		"":                     "attachment",
	}
	for input, want := range tests {
		if got := safeFilename(input); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestOutputFilename(t *testing.T) {
	got := outputFilename("123", attachment{ID: "456", Filename: "photo.png"})
	if want := "123_456_photo.png"; got != want {
		t.Fatalf("outputFilename() = %q, want %q", got, want)
	}
}

func TestImageItemsFromMessage(t *testing.T) {
	msg := message{
		ID:      "123",
		Content: "source https://example.com/full/art.jpeg?large=1",
		Attachments: []attachment{
			{ID: "a1", Filename: "upload.png", URL: "https://cdn.example/upload.png", ContentType: "image/png", Size: 42},
			{ID: "a2", Filename: "clip.mp4", URL: "https://cdn.example/clip.mp4", ContentType: "video/mp4"},
		},
		Embeds: []embed{
			{Image: &embedMedia{URL: "https://example.com/embed.webp"}, Thumbnail: &embedMedia{URL: "https://example.com/embed.webp"}},
		},
		StickerItems: []stickerItem{{ID: "s1", Name: "wave", FormatType: 1}},
	}

	items := imageItemsFromMessage(msg)
	if got, want := len(items), 4; got != want {
		t.Fatalf("len(imageItemsFromMessage()) = %d, want %d: %#v", got, want, items)
	}
	if items[0].AttachmentID != "a1" || items[1].AttachmentID != "embed-1-image" || items[2].AttachmentID != "link-1" || items[3].AttachmentID != "sticker-s1" {
		t.Fatalf("unexpected item order/IDs: %#v", items)
	}
}

func TestMessageContentEnabled(t *testing.T) {
	if messageContentEnabled(0) {
		t.Fatal("zero flags must not enable message content")
	}
	if !messageContentEnabled(1 << 18) {
		t.Fatal("GATEWAY_MESSAGE_CONTENT must enable message content")
	}
	if !messageContentEnabled(1 << 19) {
		t.Fatal("GATEWAY_MESSAGE_CONTENT_LIMITED must enable message content")
	}
}

func TestWithMessageContentEnabledPreservesFlags(t *testing.T) {
	const existing = 1 << 6
	got := withMessageContentEnabled(existing)
	if got&existing == 0 {
		t.Fatal("existing application flags were not preserved")
	}
	if got&(1<<19) == 0 {
		t.Fatal("limited message content flag was not enabled")
	}
}

func TestDownloadOneUsesAlternateURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/missing" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte("image-data"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "image.png")
	item := planItem{Filename: "image.png", URL: server.URL + "/missing", AlternateURLs: []string{server.URL + "/available"}, Size: 10}
	if err := downloadOne(context.Background(), server.Client(), item, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "image-data"; got != want {
		t.Fatalf("downloaded data = %q, want %q", got, want)
	}
}

package main

import "testing"

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

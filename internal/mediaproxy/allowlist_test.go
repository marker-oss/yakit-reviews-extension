// internal/mediaproxy/allowlist_test.go
package mediaproxy

import "testing"

func TestHostAllowed(t *testing.T) {
	allow := []string{"wbbasket.ru", "images.wildberries.ru", "avatars.mds.yandex.net"}
	ok := []string{
		"https://basket-12.wbbasket.ru/vol1/part.jpg",
		"https://images.wildberries.ru/x.jpg",
		"https://avatars.mds.yandex.net/get-marketpic/1/x/orig",
	}
	for _, u := range ok {
		if !HostAllowed(u, allow) {
			t.Errorf("HostAllowed(%q) = false, want true", u)
		}
	}
	bad := []string{
		"https://evil.com/x.jpg",
		"http://wbbasket.ru.evil.com/x", // suffix-spoof
		"https://localhost/x",           // SSRF
		"ftp://wbbasket.ru/x",           // wrong scheme
		"not a url",
	}
	for _, u := range bad {
		if HostAllowed(u, allow) {
			t.Errorf("HostAllowed(%q) = true, want false", u)
		}
	}
}

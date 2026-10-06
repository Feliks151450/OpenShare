package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("read random bytes for uuid: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hex.EncodeToString(raw[0:4]),
		hex.EncodeToString(raw[4:6]),
		hex.EncodeToString(raw[6:8]),
		hex.EncodeToString(raw[8:10]),
		hex.EncodeToString(raw[10:16]),
	), nil
}

// MustNewID 同 NewID，但随机源失败时 panic（仅用于无法返回错误的辅助场景，
// 如构造临时文件名；随机源失败在正常运行中不可能发生）。
func MustNewID() string {
	id, err := NewID()
	if err != nil {
		panic(fmt.Sprintf("identity.MustNewID: %v", err))
	}
	return id
}

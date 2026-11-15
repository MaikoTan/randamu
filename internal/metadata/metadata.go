// Package metadata 将 randamu 的来源信息内嵌进图片文件，供日后的管理工具识别。
//
// 三种格式各自利用容器层面的扩展槽位写入纯文本元信息，绝不触碰压缩后的
// 图像数据：PNG 用 tEXt 块、JPEG 用 COM 段、WebP 用 XMP 块。写入后文件
// 仍可被标准解码器正常解码。
package metadata

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
)

// Marker 是内嵌元信息的固定前缀，用于在文件中快速定位。
const Marker = "randamu:v1:"

// pngSignature 是 PNG 文件开头的 8 字节魔数。
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// ErrNotFound 表示文件中没有本包写入的元信息。
var ErrNotFound = errors.New("metadata: marker not found")

// ErrUnsupported 表示文件格式不是 JPEG/PNG/WebP。
var ErrUnsupported = errors.New("metadata: unsupported image format")

// Info 是写入图片文件的内嵌来源元信息。
type Info struct {
	Source       string   `json:"source"`        // 图源，如 pixiv:bookmarks / lolicon / konachan / danbooru
	ID           int64    `json:"id"`            // 作品 ID（illust id / post id / pid）
	Page         int      `json:"page"`          // 第几页，从 0 开始
	Title        string   `json:"title,omitempty"`
	Author       string   `json:"author,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	URL          string   `json:"url,omitempty"` // 原图下载地址
	DownloadedAt string   `json:"downloaded_at"` // RFC3339 UTC
}

// maxTags 限制写入的标签数量，避免元信息撑大文件。
const maxTags = 20

// Payload 返回 Marker 加上紧凑 JSON 的文本形式。Tags 只保留前 20 个。
func (i Info) Payload() string {
	if len(i.Tags) > maxTags {
		i.Tags = i.Tags[:maxTags]
	}
	// json.Marshal 不会失败：Info 只含基本类型与字符串切片。
	b, _ := json.Marshal(i)
	return Marker + string(b)
}

// Write 读取 path 指向的图片，内嵌 info 后原子地写回。
func Write(path string, info Info) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	payload := []byte(info.Payload())

	// 先移除之前写入的同类块，避免重复 Write 不断累积（文件膨胀、
	// Detect 读到旧值）。移除后再插入，保证文件内只有一份元信息。
	var out []byte
	switch detectFormat(data) {
	case formatPNG:
		var stripped []byte
		if stripped, err = stripPNG(data); err == nil {
			out, err = insertPNG(stripped, payload)
		}
	case formatJPEG:
		var stripped []byte
		if stripped, err = stripJPEG(data); err == nil {
			out, err = insertJPEG(stripped, payload)
		}
	case formatWebP:
		var stripped []byte
		if stripped, err = stripWebP(data); err == nil {
			out, err = insertWebP(stripped, payload)
		}
	default:
		return ErrUnsupported
	}
	if err != nil {
		return err
	}
	return writeAtomic(path, out)
}

// Detect 读取 path 并提取其中内嵌的 Info。
func Detect(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	switch detectFormat(data) {
	case formatPNG, formatJPEG, formatWebP:
	default:
		return Info{}, ErrUnsupported
	}
	return extract(data)
}

type format int

const (
	formatUnknown format = iota
	formatPNG
	formatJPEG
	formatWebP
)

// detectFormat 通过魔数判断图片格式。
func detectFormat(b []byte) format {
	switch {
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return formatPNG
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return formatJPEG
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return formatWebP
	}
	return formatUnknown
}

// stripPNG 去掉本包此前写入的 tEXt 块（keyword 为 "Randamu"），
// 其余块原样保留。解析失败时原样返回，交由后续插入步骤报错。
func stripPNG(data []byte) ([]byte, error) {
	const sigLen = 8
	if len(data) < sigLen || !bytes.Equal(data[:sigLen], pngSignature) {
		return data, nil
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:sigLen]...)
	pos := sigLen
	for pos+12 <= len(data) {
		n := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		end := pos + 12 + n
		if end > len(data) {
			// 结构异常，放弃剥离，保持原文件字节。
			return data, nil
		}
		chunk := data[pos:end]
		if string(chunk[4:8]) == "tEXt" && bytes.HasPrefix(chunk[8:], []byte("Randamu\x00")) {
			pos = end
			continue
		}
		out = append(out, chunk...)
		pos = end
		if string(chunk[4:8]) == "IEND" {
			break
		}
	}
	if pos < len(data) {
		out = append(out, data[pos:]...)
	}
	return out, nil
}

// stripJPEG 去掉此前插入的 COM 段（含 Marker 的），其余字节原样保留。
// 只扫描熵编码开始（SOS，0xDA）之前的段链，避免误解析像素数据。
func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return data, nil
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:2]...)
	pos := 2
	for pos+1 < len(data) && data[pos] == 0xFF {
		marker := data[pos+1]
		// 无长度的独立标记：TEM 与 RSTn。
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			out = append(out, data[pos:pos+2]...)
			pos += 2
			continue
		}
		// SOS 之后是熵编码数据，不再解析，剩余字节原样拷贝。
		if marker == 0xDA || marker == 0xD9 {
			break
		}
		if pos+4 > len(data) {
			return data, nil
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		end := pos + 2 + segLen
		if segLen < 2 || end > len(data) {
			return data, nil
		}
		seg := data[pos:end]
		if marker == 0xFE && bytes.Contains(seg[4:], []byte(Marker)) {
			pos = end
			continue
		}
		out = append(out, seg...)
		pos = end
	}
	out = append(out, data[pos:]...)
	return out, nil
}

// stripWebP 去掉此前追加的 XMP 块（含 Marker 的），重算 RIFF 长度。
func stripWebP(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return data, nil
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:12]...)
	pos := 12
	for pos+8 <= len(data) {
		n := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		size := 8 + n + (n & 1) // 奇数长度带 1 字节填充
		end := pos + size
		if end > len(data) {
			return data, nil
		}
		chunk := data[pos:end]
		if string(chunk[0:4]) == "XMP " && bytes.Contains(chunk[8:], []byte(Marker)) {
			pos = end
			continue
		}
		out = append(out, chunk...)
		pos = end
	}
	out = append(out, data[pos:]...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

// insertPNG 在 IHDR 之后插入一个 tEXt 块。
//
// tEXt 的 keyword 按规范是 Latin-1，此处 text 却是 UTF-8；实践中解码器
// 直接按字节读取，能正常显示中文，故不做转码。
func insertPNG(data, payload []byte) ([]byte, error) {
	const sigLen = 8
	if len(data) < sigLen+8 {
		return nil, fmt.Errorf("metadata: truncated PNG")
	}
	// IHDR 紧随签名，长度字段位于 [8:12]，整块长度为 12+len。
	ihdrLen := int(binary.BigEndian.Uint32(data[sigLen : sigLen+4]))
	end := sigLen + 12 + ihdrLen
	if end > len(data) || string(data[sigLen+4:sigLen+8]) != "IHDR" {
		return nil, fmt.Errorf("metadata: malformed PNG header")
	}

	chunk := make([]byte, 0, 12+len("Randamu")+1+len(payload))
	text := make([]byte, 0, len("Randamu")+1+len(payload))
	text = append(text, "Randamu"...)
	text = append(text, 0)
	text = append(text, payload...)

	var head [8]byte
	binary.BigEndian.PutUint32(head[0:], uint32(len(text)))
	copy(head[4:], "tEXt")
	chunk = append(chunk, head[:]...)
	chunk = append(chunk, text...)
	crc := crc32.NewIEEE()
	crc.Write([]byte("tEXt"))
	crc.Write(text)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	chunk = append(chunk, sum[:]...)

	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:end]...)
	out = append(out, chunk...)
	out = append(out, data[end:]...)
	return out, nil
}

// insertJPEG 在 SOI 之后的 APPn/COM 段之后插入一个 COM 段。
//
// 曾经考虑用 XMP（APP1）承载结构化元信息，但 COM 段更简单，且工具可直接
// 解析其中的纯文本，因此采用 COM。
func insertJPEG(data, payload []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("metadata: truncated JPEG")
	}
	pos := 2
	for pos+4 <= len(data) && data[pos] == 0xFF {
		marker := data[pos+1]
		// APPn（E0-EF）与 COM（FE）带 2 字节长度，需要跳过。
		if (marker >= 0xE0 && marker <= 0xEF) || marker == 0xFE {
			segLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
			if segLen < 2 || pos+2+segLen > len(data) {
				return nil, fmt.Errorf("metadata: malformed JPEG segment")
			}
			pos += 2 + segLen
			continue
		}
		// 无长度的独立标记（TEM、RSTn）罕见地出现在此处，跳过 2 字节。
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			pos += 2
			continue
		}
		break
	}

	segLen := len(payload) + 2
	if segLen > 0xFFFF {
		return nil, fmt.Errorf("metadata: payload too large for JPEG COM")
	}
	seg := make([]byte, 0, 4+len(payload))
	seg = append(seg, 0xFF, 0xFE, byte(segLen>>8), byte(segLen))
	seg = append(seg, payload...)

	out := make([]byte, 0, len(data)+len(seg))
	out = append(out, data[:pos]...)
	out = append(out, seg...)
	out = append(out, data[pos:]...)
	return out, nil
}

// insertWebP 在 RIFF 块列表末尾追加一个 XMP 块，并更新 RIFF 总长度。
//
// 务实做法：直接放 randamu 文本而非标准 XMP 包装，供我们自己的管理端解析；
// libwebp 会忽略不认识的块，仍能正常解码。
func insertWebP(data, payload []byte) ([]byte, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("metadata: truncated WebP")
	}
	chunkLen := len(payload)
	pad := chunkLen & 1
	chunk := make([]byte, 0, 8+chunkLen+pad)
	chunk = append(chunk, "XMP "...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(chunkLen))
	chunk = append(chunk, size[:]...)
	chunk = append(chunk, payload...)
	if pad == 1 {
		chunk = append(chunk, 0)
	}

	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data...)
	out = append(out, chunk...)
	// RIFF 长度字段（字节 4-7）等于文件总长减去 8，小端序。
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

// extract 定位 Marker 后解析其后的 JSON 对象。
func extract(data []byte) (Info, error) {
	idx := bytes.Index(data, []byte(Marker))
	if idx < 0 {
		return Info{}, ErrNotFound
	}
	rest := data[idx+len(Marker):]
	start := bytes.IndexByte(rest, '{')
	if start < 0 {
		return Info{}, fmt.Errorf("metadata: malformed JSON payload")
	}
	end := matchingBrace(rest[start:])
	if end < 0 {
		return Info{}, fmt.Errorf("metadata: unterminated JSON payload")
	}
	var info Info
	if err := json.Unmarshal(rest[start:start+end], &info); err != nil {
		return Info{}, fmt.Errorf("metadata: decode payload: %w", err)
	}
	return info, nil
}

// matchingBrace 返回与首字符 '{' 配对的 '}' 的下标（含），找不到返回 -1。
// 扫描时跳过字符串内部的括号，正确处理转义。
func matchingBrace(b []byte) int {
	depth := 0
	inString := false
	escaped := false
	for i, c := range b {
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// writeAtomic 先写同目录临时文件，再 rename 覆盖目标，并保留原文件权限。
func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".randamu-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

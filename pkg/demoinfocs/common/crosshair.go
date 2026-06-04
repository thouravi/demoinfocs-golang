package common

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Crosshair represents the decoded settings from a CS:GO / CS2 crosshair share code (e.g. "CSGO-...").
// These values correspond closely to the cl_crosshair* convars.
type Crosshair struct {
	Length  float64 `json:"length"`  // cl_crosshairsize
	Gap     float64 `json:"gap"`     // cl_crosshairgap (can be negative)
	Outline float64 `json:"outline"` // cl_crosshair_outlinethickness (when enabled)

	Red   int `json:"red"`
	Green int `json:"green"`
	Blue  int `json:"blue"`
	Alpha int `json:"alpha"`

	// Color is the color preset index (0-6 or so); when set the rgb may be ignored by game depending on cl_crosshaircolor.
	Color int `json:"color"`

	// Style: 0-4 classic variants, 5 legacy etc. See game docs.
	Style int `json:"style"`

	Thickness float64 `json:"thickness"` // cl_crosshairthickness

	CenterDotEnabled         bool `json:"centerDotEnabled"`
	OutlineEnabled           bool `json:"outlineEnabled"`
	AlphaEnabled             bool `json:"alphaEnabled"`
	TStyleEnabled            bool `json:"tStyleEnabled"`            // top line missing?
	DeployedWeaponGapEnabled bool `json:"deployedWeaponGapEnabled"` // gap use weapon value
	FollowRecoil             bool `json:"followRecoil"`             // CS2 recoil crosshair

	SplitDistance     int     `json:"splitDistance"`
	FixedCrosshairGap float64 `json:"fixedCrosshairGap"`
	InnerSplitAlpha   float64 `json:"innerSplitAlpha"`
	OuterSplitAlpha   float64 `json:"outerSplitAlpha"`
	SplitSizeRatio    float64 `json:"splitSizeRatio"`
}

// DecodeCrosshairShareCode decodes a crosshair share code into its component settings.
// Returns error for invalid format or checksum mismatch.
// The code format is the same for CS:GO and CS2 (e.g. "CSGO-jvnbx-S3xFK-iEJXD-Y27Nd-AO6FP").
func DecodeCrosshairShareCode(shareCode string) (*Crosshair, error) {
	bytes, err := shareCodeToBytes(shareCode)
	if err != nil {
		return nil, err
	}
	if len(bytes) < 15 {
		return nil, ErrInvalidCrosshairShareCode
	}

	// checksum: bytes[0] == sum(bytes[1:]) % 256
	sum := 0
	for i := 1; i < len(bytes); i++ {
		sum += int(bytes[i])
	}
	if bytes[0] != byte(sum%256) {
		return nil, ErrInvalidCrosshairShareCode
	}

	c := &Crosshair{
		Gap:     float64(int8(bytes[2])) / 10,
		Outline: float64(bytes[3]) / 2,
		Red:     int(bytes[4]),
		Green:   int(bytes[5]),
		Blue:    int(bytes[6]),
		Alpha:   int(bytes[7]),

		SplitDistance: int(bytes[8] & 0x7f),
		FollowRecoil:  (bytes[8] >> 7 & 1) == 1,

		FixedCrosshairGap: float64(int8(bytes[9])) / 10,

		Color:           int(bytes[10] & 7),
		OutlineEnabled:  (bytes[10] & 8) == 8,
		InnerSplitAlpha: float64(bytes[10]>>4) / 10,

		OuterSplitAlpha: float64(bytes[11]&0xf) / 10,
		SplitSizeRatio:  float64(bytes[11]>>4) / 10,

		Thickness: float64(bytes[12]) / 10,

		CenterDotEnabled:         ((bytes[13] >> 4) & 1) == 1,
		DeployedWeaponGapEnabled: ((bytes[13] >> 5) & 1) == 1,
		AlphaEnabled:             ((bytes[13] >> 6) & 1) == 1,
		TStyleEnabled:            ((bytes[13] >> 7) & 1) == 1,
		Style:                    int((bytes[13] & 0xf) >> 1),

		Length: float64(((int(bytes[15])&0x1f)<<8)+int(bytes[14])) / 10,
	}
	return c, nil
}

func (c *Crosshair) String() string {
	if c == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Crosshair{style:%d len:%.1f gap:%.1f thick:%.1f rgb:(%d,%d,%d) alpha:%d outline:%v dot:%v}",
		c.Style, c.Length, c.Gap, c.Thickness, c.Red, c.Green, c.Blue, c.Alpha, c.OutlineEnabled, c.CenterDotEnabled)
}

// ErrInvalidShareCode is returned when the share code string does not match the expected format.
var ErrInvalidShareCode = errors.New("invalid share code")

// ErrInvalidCrosshairShareCode is returned for crosshair codes that fail length/checksum validation.
var ErrInvalidCrosshairShareCode = errors.New("invalid crosshair share code")

const crosshairDictionary = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789"

func shareCodeToBytes(shareCode string) ([]byte, error) {
	// Normalize: remove CSGO prefix and all dashes (some codes may omit dashes in theory)
	normalized := strings.ReplaceAll(shareCode, "CSGO", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.TrimSpace(normalized)

	if len(normalized) != 25 {
		return nil, ErrInvalidShareCode
	}

	// Reverse chars (per reference impl)
	chars := []rune(normalized)
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}

	total := big.NewInt(0)
	dictLen := big.NewInt(int64(len(crosshairDictionary)))
	for _, c := range chars {
		idx := strings.IndexRune(crosshairDictionary, c)
		if idx < 0 {
			return nil, ErrInvalidShareCode
		}
		total.Mul(total, dictLen)
		total.Add(total, big.NewInt(int64(idx)))
	}

	// 18 bytes = 36 hex digits
	hexStr := fmt.Sprintf("%036x", total)
	if len(hexStr) > 36 {
		hexStr = hexStr[len(hexStr)-36:]
	}
	if len(hexStr) < 36 {
		hexStr = strings.Repeat("0", 36-len(hexStr)) + hexStr
	}

	bytes := make([]byte, 18)
	for i := 0; i < 18; i++ {
		b, err := strconv.ParseUint(hexStr[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, ErrInvalidShareCode
		}
		bytes[i] = byte(b)
	}
	return bytes, nil
}

// ToConsoleCommands returns a multi-line string of cl_crosshair* commands that would recreate this crosshair in-game.
func (c *Crosshair) ToConsoleCommands() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf(`cl_crosshair_drawoutline "%d"
cl_crosshair_dynamic_maxdist_splitratio "%.1f"
cl_crosshair_dynamic_splitalpha_innermod "%.1f"
cl_crosshair_dynamic_splitalpha_outermod "%.1f"
cl_crosshair_dynamic_splitdist "%d"
cl_crosshair_outlinethickness "%.1f"
cl_crosshair_t "%d"
cl_crosshairalpha "%d"
cl_crosshaircolor "%d"
cl_crosshaircolor_b "%d"
cl_crosshaircolor_g "%d"
cl_crosshaircolor_r "%d"
cl_crosshairdot "%d"
cl_crosshairgap "%.1f"
cl_crosshairgap_useweaponvalue "%d"
cl_crosshairsize "%.1f"
cl_crosshairstyle "%d"
cl_crosshairthickness "%.1f"
cl_crosshairusealpha "%d"
cl_fixedcrosshairgap "%.1f"
cl_crosshair_recoil "%d"
`, b2i(c.OutlineEnabled), c.SplitSizeRatio, c.InnerSplitAlpha, c.OuterSplitAlpha, c.SplitDistance,
		c.Outline, b2i(c.TStyleEnabled), c.Alpha, c.Color, c.Blue, c.Green, c.Red,
		b2i(c.CenterDotEnabled), c.Gap, b2i(c.DeployedWeaponGapEnabled), c.Length, c.Style, c.Thickness,
		b2i(c.AlphaEnabled), c.FixedCrosshairGap, b2i(c.FollowRecoil))
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// EncodeCrosshairShareCode encodes a Crosshair settings struct back into a share code string.
// This allows round-tripping or generating share codes from custom settings.
func EncodeCrosshairShareCode(c *Crosshair) (string, error) {
	if c == nil {
		return "", errors.New("nil crosshair")
	}
	bytes := []byte{
		0, // checksum placeholder
		1, // ?
		byte((int(c.Gap * 10)) & 0xff),
		byte(c.Outline * 2),
		byte(c.Red),
		byte(c.Green),
		byte(c.Blue),
		byte(c.Alpha),
		byte((c.SplitDistance & 0x7f) | (b2i(c.FollowRecoil) << 7)),
		byte((int(c.FixedCrosshairGap * 10)) & 0xff),
		byte((c.Color & 7) | (b2i(c.OutlineEnabled) << 3) | (int(c.InnerSplitAlpha*10) << 4)),
		byte((int(c.OuterSplitAlpha * 10)) | (int(c.SplitSizeRatio*10) << 4)),
		byte(c.Thickness * 10),
		byte((c.Style << 1) |
			(b2i(c.CenterDotEnabled) << 4) |
			(b2i(c.DeployedWeaponGapEnabled) << 5) |
			(b2i(c.AlphaEnabled) << 6) |
			(b2i(c.TStyleEnabled) << 7)),
		byte(int(c.Length*10) & 0xff),
		byte((int(c.Length*10) >> 8) & 0x1f),
		0, 0,
	}
	// set checksum
	sum := 0
	for i := 1; i < len(bytes); i++ {
		sum += int(bytes[i])
	}
	bytes[0] = byte(sum & 0xff)

	return bytesToShareCode(bytes), nil
}

func bytesToShareCode(bytes []byte) string {
	hexStr := ""
	for _, b := range bytes {
		hexStr += fmt.Sprintf("%02x", b)
	}
	total := big.NewInt(0)
	total.SetString(hexStr, 16)

	dictLen := big.NewInt(int64(len(crosshairDictionary)))
	chars := ""
	for i := 0; i < 25; i++ {
		rem := new(big.Int).Mod(total, dictLen)
		idx := int(rem.Int64())
		chars += string(crosshairDictionary[idx])
		total.Div(total, dictLen)
	}
	// chars built LSB-first; slicing directly matches the reference encode (shareCodeToBytes reverses on decode)
	s := chars
	return fmt.Sprintf("CSGO-%s-%s-%s-%s-%s", s[0:5], s[5:10], s[10:15], s[15:20], s[20:25])
}

package color

import "strings"

// ANSI SGR (Select Graphic Rendition) escape codes, 标准库实现，无第三方依赖。
const (
	esc     = "\x1b["
	sgrEnd  = "m"
	reset   = esc + "0" + sgrEnd
	bold    = "1"
	// 前景色
	fgBlack   = "30"
	fgRed     = "31"
	fgGreen   = "32"
	fgYellow  = "33"
	fgBlue    = "34"
	fgMagenta = "35"
	fgCyan    = "36"
	fgWhite   = "37"
	fgHiBlack = "90"
	fgHiWhite = "97"
	// 背景色
	bgBlack   = "40"
	bgRed     = "41"
	bgGreen   = "42"
	bgYellow  = "43"
	bgBlue    = "44"
	bgMagenta = "45"
	bgCyan    = "46"
	bgWhite   = "47"
)

const (
	// NoColor is no color for both foreground and background.
	NoColor Color = iota
	// FgBlack is the foreground color black.
	FgBlack
	// FgRed is the foreground color red.
	FgRed
	// FgGreen is the foreground color green.
	FgGreen
	// FgYellow is the foreground color yellow.
	FgYellow
	// FgBlue is the foreground color blue.
	FgBlue
	// FgMagenta is the foreground color magenta.
	FgMagenta
	// FgCyan is the foreground color cyan.
	FgCyan
	// FgWhite is the foreground color white.
	FgWhite

	// BgBlack is the background color black.
	BgBlack
	// BgRed is the background color red.
	BgRed
	// BgGreen is the background color green.
	BgGreen
	// BgYellow is the background color yellow.
	BgYellow
	// BgBlue is the background color blue.
	BgBlue
	// BgMagenta is the background color magenta.
	BgMagenta
	// BgCyan is the background color cyan.
	BgCyan
	// BgWhite is the background color white.
	BgWhite
)

// colors 保存每个 Color 对应的 ANSI SGR 参数序列（与 fatih/color 语义保持一致）。
var colors = map[Color][]string{
	FgBlack:   {bold, fgBlack},
	FgRed:     {bold, fgRed},
	FgGreen:   {bold, fgGreen},
	FgYellow:  {bold, fgYellow},
	FgBlue:    {bold, fgBlue},
	FgMagenta: {bold, fgMagenta},
	FgCyan:    {bold, fgCyan},
	FgWhite:   {bold, fgWhite},
	BgBlack:   {bold, fgHiWhite, bgBlack},
	BgRed:     {bold, fgHiWhite, bgRed},
	BgGreen:   {bold, fgHiWhite, bgGreen},
	BgYellow:  {bold, fgHiBlack, bgYellow},
	BgBlue:    {bold, fgHiWhite, bgBlue},
	BgMagenta: {bold, fgHiWhite, bgMagenta},
	BgCyan:    {bold, fgHiWhite, bgCyan},
	BgWhite:   {bold, fgHiBlack, bgWhite},
}

type Color uint32

// WithColor returns a string with the given color applied.
func WithColor(text string, colour Color) string {
	attrs, ok := colors[colour]
	if !ok {
		return text
	}

	seq := esc + joinParams(attrs) + sgrEnd
	return seq + text + reset
}

// WithColorPadding returns a string with the given color applied with leading and trailing spaces.
func WithColorPadding(text string, colour Color) string {
	return WithColor(" "+text+" ", colour)
}

// joinParams 拼接 SGR 参数，如 "1;34"。
func joinParams(params []string) string {
	var b strings.Builder
	for i, p := range params {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(p)
	}
	return b.String()
}

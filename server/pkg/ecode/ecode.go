package ecode

import (
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/pkg/errors"
)

var (
	_messages        atomic.Value
	_codes           = map[int]struct{}{}
	currentLangIndex int32 = 0
	supportedLangs         = map[string]int32{"zh-CN": 0, "en-US": 1}
)

// EcodeMessageMap 全局错误码消息映射（由业务层填充）。
var EcodeMessageMap = map[int][]string{}

// Register 注册错误码消息映射。
func Register(cm map[int][]string) {
	_messages.Store(cm)
}

// SetLanguage 设置当前语言。
func SetLanguage(lang string) {
	if index, ok := supportedLangs[lang]; ok {
		atomic.StoreInt32(&currentLangIndex, index)
	}
}

// GetLanguage 获取当前语言。
func GetLanguage() string {
	index := atomic.LoadInt32(&currentLangIndex)
	for lang, idx := range supportedLangs {
		if idx == index {
			return lang
		}
	}
	return "zh-CN"
}

// Codes 错误码接口。
type Codes interface {
	Error() string
	Code() int
	Message() string
	MessageEn() string
	MessageForLang() string
	Details() []interface{}
}

// Code 业务错误码类型。
type Code int

func (e Code) Error() string            { return strconv.FormatInt(int64(e), 10) }
func (e Code) Code() int                { return int(e) }
func (e Code) Message() string          { return e.getMessage(0) }
func (e Code) MessageEn() string        { return e.getMessage(1) }
func (e Code) Details() []interface{}   { return nil }
func (e Code) MessageForLang() string   { return e.getMessage(int(atomic.LoadInt32(&currentLangIndex))) }

func (e Code) getMessage(langIndex int) string {
	if cm, ok := _messages.Load().(map[int][]string); ok {
		if messages, ok := cm[e.Code()]; ok {
			if langIndex < len(messages) && messages[langIndex] != "" {
				return messages[langIndex]
			}
			if len(messages) > 0 && messages[0] != "" {
				return messages[0]
			}
		}
	}
	return e.Error()
}

func Int(i int) Code { return Code(i) }

// Cause 从 error 链中提取错误码。
func Cause(e error) Codes {
	if e == nil {
		return OK
	}
	ec, ok := errors.Cause(e).(Codes)
	if ok {
		return ec
	}
	if c, ok := e.(Codes); ok {
		return c
	}
	return String(e.Error())
}

// String 从字符串错误信息中解析错误码。
func String(e string) Code {
	if e == "" {
		return OK
	}
	i, err := strconv.Atoi(e)
	if err != nil {
		return ServerErr
	}
	return Code(i)
}

// Equal 判断两个错误码是否相等。
func Equal(a, b Codes) bool {
	if a == nil {
		a = OK
	}
	if b == nil {
		b = OK
	}
	return a.Code() == b.Code()
}

func EqualError(code Codes, err error) bool {
	return Cause(err).Code() == code.Code()
}

func New(e int) Code { return Code(e) }

func panicf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

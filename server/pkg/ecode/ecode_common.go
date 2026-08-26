package ecode

// 通用错误码定义。
var (
	OK           = Code(0)
	ServerErr    = Code(1)
	ParamErr     = Code(2)
	NotFound     = Code(3)
	Unauthorized = Code(4)
	Forbidden    = Code(5)
	Conflict     = Code(6)
	Internal     = Code(7)
)

func init() {
	EcodeMessageMap[0] = []string{"成功", "Success"}
	EcodeMessageMap[1] = []string{"服务器内部错误", "Internal server error"}
	EcodeMessageMap[2] = []string{"参数错误", "Invalid parameter"}
	EcodeMessageMap[3] = []string{"资源不存在", "Resource not found"}
	EcodeMessageMap[4] = []string{"未授权", "Unauthorized"}
	EcodeMessageMap[5] = []string{"禁止访问", "Forbidden"}
	EcodeMessageMap[6] = []string{"资源冲突", "Conflict"}
	EcodeMessageMap[7] = []string{"内部错误", "Internal error"}
	Register(EcodeMessageMap)
}

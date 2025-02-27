package msg

import (
	"github.com/astaxie/beego/validation"
	"net/http"
)

const (
	SuccessCode = 1
	ErrCode = 0
)

// API Response 基础序列化器
type Response struct {
	Code  int         `json:"code"`
	Data  interface{} `json:"data"`
	Error string      `json:"error"`
}

// Err 通用错误处理
func ErrResp(errStr string) (int,Response) {
	res := Response{
		Code: ErrCode,
		Data: nil,
		Error:  errStr,
	}
	return http.StatusOK, res
}

// SuccessResp 通用处理
func SuccessResp(data interface{}) (int,Response) {
	res := Response{
		Code:  SuccessCode,
		Data:  data,
		Error: "",
	}
	return http.StatusOK, res
}

func DealValidError(valid validation.Validation) string {
	errStr := "参数校验不通过:"
	for _, err := range valid.Errors {
		errStr += err.Message + ";"
	}
	return errStr
}


package engine

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zhTrans "github.com/go-playground/validator/v10/translations/zh"
	jsoniter "github.com/json-iterator/go"
	"go_client/pkg/status"
	"io"
	"net/http"
)

var (
	ErrBadRequest = status.WrapperE(http.StatusBadRequest, "Bad Request")
)

var (
	validate = validator.New()
	trans    ut.Translator
)

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	trans, _ = ut.New(zh.New()).GetTranslator("zh")

	if err := zhTrans.RegisterDefaultTranslations(validate, trans); err != nil {
		panic(err)
	}
}

func registrationFunc(tag string, translation string, override bool) validator.RegisterTranslationsFunc {
	return func(ut ut.Translator) error {
		return ut.Add(tag, translation, override)
	}
}

func translateFunc(ut ut.Translator, fe validator.FieldError) string {
	t, err := ut.T(fe.Tag(), fe.Field())
	if err != nil {
		return fe.(error).Error()
	}
	return t
}

func Validate(s any) error {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var vErrs validator.ValidationErrors
	if errors.As(err, &vErrs) {
		for i := range vErrs {
			return status.WrapperE(http.StatusBadRequest, vErrs[i].Translate(trans))
		}
	}
	return status.Wrapper(http.StatusBadRequest, err)
}

func Bind[T any](val *T, r io.Reader) error {

	if err := jsoniter.ConfigFastest.NewDecoder(r).Decode(val); err != nil {
		return ErrBadRequest
	}
	return Validate(val)
}

func BindQuery[T any](val *T, r *http.Request) error {

	if err := binding.Query.Bind(r, val); err != nil {
		return ErrBadRequest
	}
	return Validate(val)
}

func BindParams[T any](val *T, params gin.Params) error {
	m := make(map[string][]string)
	for i := range params {
		m[params[i].Key] = []string{params[i].Value}
	}

	if err := binding.Uri.BindUri(m, val); err != nil {
		return ErrBadRequest
	}
	return Validate(val)
}

type Paging struct {
	Offset uint `json:"offset" form:"offset" validate:"gte=0"`
	Limit  uint `json:"limit" form:"limit" validate:"gte=5,lte=20"`
}

func (r Paging) SumOffset() int {
	return int(r.Offset * r.Limit)
}

type PagingAck[T any] struct {
	Total int64 `json:"total"`
	List  []*T  `json:"list,omitempty"`
}

type SessionAction struct {
	SessionID string `json:"sessionID" uri:"sessionID" validate:"required"`
}

// 创建会话Req
type CreateSessionReq struct {
	ID         string `json:"id" validate:"required"`
	RtspURL    string `json:"rtspURL"  validate:"required"` // 摄像头播放地址URL
	Width      int    `json:"width" validate:"gte=0"`       //  宽
	Height     int    `json:"height" validate:"gte=0"`      //  高
	RetryTimes int    `json:"retryTimes" validate:"gt=0"`   // 读帧失败重试次数
	Framerate  int    `json:"framerate" validate:"gte=0"`   // 帧率
}

type GetSessionDescByIDResp struct {
	Exists  bool        `json:"exists"`
	Session SessionDesc `json:"desc"`
}

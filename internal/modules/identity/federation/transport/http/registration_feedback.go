package federationhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/shared/passwordpolicy"
	"golang.org/x/crypto/bcrypt"
)

// Only the registered first-party form origins can read credentialed results.
// Passwords and authority cookies stay between the browser and the authority.
func (s *Server) registrationJSON(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != s.issuer.Path+"/login/register" || r.Header.Get("Accept") != "application/json" {
		return false
	}
	for _, form := range s.forms {
		if r.Header.Get("Origin") == form.Scheme+"://"+form.Host {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Content-Type", "application/json")
			return true
		}
	}
	return false
}

func registrationError(err error) (int, string, string, string) {
	switch {
	case errors.Is(err, userauthapp.ErrEmailExists):
		return 409, "email_exists", "email", "该邮箱已经注册，请直接登录或找回密码。"
	case errors.Is(err, userauthapp.ErrInvalidEmail):
		return 422, "invalid_email", "email", "邮箱地址格式不正确，请检查后重试。"
	case errors.Is(err, userauthapp.ErrVerifyCodeInvalid):
		return 422, "invalid_code", "code", "邮箱验证码错误，请重新输入最新收到的验证码。"
	case errors.Is(err, userauthapp.ErrVerifyCodeExpired):
		return 422, "expired_code", "code", "邮箱验证码已过期，请重新发送。"
	case errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded):
		return 429, "attempts_exceeded", "code", "验证码错误次数过多，请重新发送验证码。"
	case errors.Is(err, userauthapp.ErrRegistrationDisabled):
		return 403, "registration_closed", "", "暂未开放注册，请稍后再试。"
	case errors.Is(err, userauthapp.ErrInvalidPassword):
		return 422, "password_mismatch", "password_confirm", "两次输入的密码不一致，请检查确认密码。"
	case errors.Is(err, bcrypt.ErrPasswordTooLong):
		return 422, "password_too_long", "password", "密码过长，请缩短后重试。"
	case errors.Is(err, passwordpolicy.ErrWeak):
		text := "密码不符合要求，请按密码下方的规则修改。"
		var detail interface{ Key() string }
		if errors.As(err, &detail) {
			switch detail.Key() {
			case "error.password_require_upper":
				text = "密码需要包含至少一个大写字母。"
			case "error.password_require_lower":
				text = "密码需要包含至少一个小写字母。"
			case "error.password_require_number":
				text = "密码需要包含至少一个数字。"
			case "error.password_require_special":
				text = "密码需要包含至少一个特殊符号。"
			}
		}
		return 422, "weak_password", "password", text
	}
	return 503, "registration_unavailable", "", "注册服务暂时无法完成请求，请稍后重试。"
}

func writeRegistrationError(w http.ResponseWriter, status int, code, field, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "field": field, "error": message})
}

func registrationProtocolError(action string) (string, string) {
	if strings.Contains(action, "rate") {
		return "too_frequent", "操作较频繁，请稍等一分钟后重试。"
	}
	if action == "client_challenge" || action == "captcha" {
		return "captcha_required", "拼图验证已失效，请重新完成拼图后提交。"
	}
	return "flow_expired", "注册页面验证已失效，请重新打开注册页。"
}

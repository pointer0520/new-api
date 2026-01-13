package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

// TurnstileCheck Cloudflare 人机校验
func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否启用了Turnstile校验功能
		if common.TurnstileCheckEnabled {
			// 获取当前会话
			session := sessions.Default(c)
			// 检查用户是否已经通过了turnstile验证
			turnstileChecked := session.Get("turnstile")
			if turnstileChecked != nil {
				// 如果已验证，则继续执行后续处理器
				c.Next()
				return
			}

			// 从请求参数中获取turnstile响应token
			response := c.Query("turnstile")
			if response == "" {
				// 如果没有提供token，返回错误
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile token 为空",
				})
				c.Abort()
				return
			}

			// 发送POST请求到Cloudflare的turnstile验证端点
			rawRes, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
				"secret":   {common.TurnstileSecretKey}, // 使用配置的密钥
				"response": {response},                  // 用户提供的响应token
				"remoteip": {c.ClientIP()},              // 客户端IP地址
			})
			if err != nil {
				// 记录错误日志并返回错误信息
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			defer rawRes.Body.Close()

			// 解析验证响应
			var res turnstileCheckResponse
			err = json.NewDecoder(rawRes.Body).Decode(&res)
			if err != nil {
				// 记录错误日志并返回错误信息
				common.SysLog(err.Error())
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}

			// 检查验证结果
			if !res.Success {
				// 如果验证失败，返回错误信息
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Turnstile 校验失败，请刷新重试！",
				})
				c.Abort()
				return
			}

			// 验证成功后，在会话中记录已验证状态
			session.Set("turnstile", true)
			err = session.Save()
			if err != nil {
				// 如果无法保存会话，返回错误信息
				c.JSON(http.StatusOK, gin.H{
					"message": "无法保存会话信息，请重试",
					"success": false,
				})
				return
			}
		}
		// 继续执行后续处理器
		c.Next()
	}
}

package controller

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/logger"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type ssoVerifyResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data *ssoUser `json:"data"`
}

type ssoUser struct {
	UserID      int64  `json:"userId"`
	UserName    string `json:"userName"`
	NickName    string `json:"nickName"`
	PhoneNumber string `json:"phonenumber"`
	Email       string `json:"email"`
	DeptID      int64  `json:"deptId"`
	CreateTime  int64  `json:"createTime"`
}

func SsoCallback(c *gin.Context) {
	ctx := c.Request.Context()
	ssoToken := strings.TrimSpace(c.Query("ssoToken"))
	if ssoToken == "" {
		logger.LogWarn(ctx, "SSO callback missing ssoToken")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "ssoToken is required",
		})
		return
	}

	logger.LogDebug(ctx, fmt.Sprintf("SSO callback received, token prefix: %s***",
		ssoToken[:min(8, len(ssoToken))]))

	upstreamUser, err := verifySSOToken(ssoToken)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("SSO token verify failed: %s", err.Error()))
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("SSO token verified, upstream user: userId=%d, username=%s, email=%s",
		upstreamUser.UserID, upstreamUser.UserName, upstreamUser.Email))
	user, err := loadOrCreateSSOUser(upstreamUser)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("SSO user load/create failed: %s", err.Error()))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "user is disabled",
		})
		return
	}

	var insertedUser model.User
	if err := model.DB.Where("username = ?", user.Username).First(&insertedUser).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户ID失败",
		})
	}

	setupLogin(&insertedUser, c)
}

func verifySSOToken(ssoToken string) (*ssoUser, error) {
	verifyURL := common.GetEnvOrDefaultString("SSO_VERIFY_URL", "http://58.215.226.11:55402/prod-api/auth/sso/verify-token")
	secretKey := common.GetEnvOrDefaultString("SSO_SECRET_KEY", "your_sso_secret_key_2026")
	timeoutSec := common.GetEnvOrDefault("SSO_VERIFY_TIMEOUT_SECONDS", 5)
	if timeoutSec <= 0 {
		timeoutSec = 5
	}

	client := http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	var lastErr error

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodGet, verifyURL, nil)
		if err != nil {
			lastErr = err
			if attempt == 0 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		query := req.URL.Query()
		query.Set("ssoToken", ssoToken)
		req.URL.RawQuery = query.Encode()

		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		req.Header.Set("X-Sign-Timestamp", ts)
		req.Header.Set("X-Sign-Value", md5Hex(ssoToken+ts+secretKey))
		req.Header.Set("CurrentEnv", "development")
		fmt.Println("=== URL")
		fmt.Println(req.URL)
		fmt.Println("signSrc=", ssoToken+ts+secretKey)
		fmt.Println("=== HEADERS")
		for k, v := range req.Header {
			fmt.Println(k, ":", v)
		}

		resp, err := client.Do(req)
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			fmt.Println("读取响应失败：", readErr)
			continue
		}
		fmt.Printf("原始响应内容：%s\n", string(bodyBytes))
		if err != nil {
			lastErr = err
			if attempt == 0 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		var verifyRes ssoVerifyResponse
		decodeErr := json.Unmarshal(bodyBytes, &verifyRes)
		_ = resp.Body.Close()
		if decodeErr != nil {
			lastErr = decodeErr
			if attempt == 0 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		fmt.Println(resp)
		if verifyRes.Code != http.StatusOK || verifyRes.Data == nil {
			if verifyRes.Msg == "" {
				verifyRes.Msg = fmt.Sprintf("sso verify failed, code=%d", verifyRes.Code)
			}
			return nil, errors.New(verifyRes.Msg)
		}

		return verifyRes.Data, nil
	}

	if lastErr == nil {
		lastErr = errors.New("sso verify request failed")
	}
	return nil, lastErr
}

func loadOrCreateSSOUser(upstreamUser *ssoUser) (*model.User, error) {
	if upstreamUser == nil {
		return nil, errors.New("empty sso user")
	}

	externalID := "sso:" + strconv.FormatInt(upstreamUser.UserID, 10)
	user := &model.User{OidcId: externalID}

	if model.IsOidcIdAlreadyTaken(externalID) {
		if err := user.FillUserByOidcId(); err != nil {
			return nil, err
		}
		return user, nil
	}

	// Reuse existing email account when possible, then bind external identity once.
	if upstreamUser.Email != "" && model.IsEmailAlreadyTaken(upstreamUser.Email) {
		user = &model.User{Email: upstreamUser.Email}
		if err := user.FillUserByEmail(); err != nil {
			return nil, err
		}
		if user.OidcId != "" && user.OidcId != externalID {
			return nil, errors.New("email is already bound to another external account")
		}
		if user.OidcId == "" {
			user.OidcId = externalID
			if err := user.Update(false); err != nil {
				return nil, err
			}
		}
		return user, nil
	}

	if !common.RegisterEnabled {
		return nil, errors.New("registration is disabled")
	}

	username, err := buildSSOUsername(upstreamUser)
	if err != nil {
		return nil, err
	}

	displayName := strings.TrimSpace(upstreamUser.NickName)
	if displayName == "" {
		displayName = strings.TrimSpace(upstreamUser.UserName)
	}
	if displayName == "" {
		displayName = "SSO User"
	}
	displayName = truncateRunes(displayName, 20)

	newUser := &model.User{
		Username:    username,
		Password:    common.GetRandomString(16),
		DisplayName: displayName,
		Email:       strings.TrimSpace(upstreamUser.Email),
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		OidcId:      externalID,
	}

	if err := newUser.Insert(0); err != nil {
		return nil, err
	}
	return newUser, nil
}

func buildSSOUsername(upstreamUser *ssoUser) (string, error) {
	base := normalizeUsername(upstreamUser.UserName)
	if base == "" {
		base = "sso_" + strconv.FormatInt(upstreamUser.UserID, 10)
	}
	base = truncateRunes(base, 20)
	if base == "" {
		base = "sso_user"
	}

	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			suffix := "_" + strconv.Itoa(i)
			candidate = truncateRunes(base, 20-len([]rune(suffix))) + suffix
		}
		exist, err := model.CheckUserExistOrDeleted(candidate, "")
		if err != nil {
			return "", err
		}
		if !exist {
			return candidate, nil
		}
	}
	return "", errors.New("failed to allocate username for sso user")
}

func normalizeUsername(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var out []rune
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			out = append(out, unicode.ToLower(r))
		}
	}
	normalized := strings.Trim(string(out), "_-")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func md5Hex(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

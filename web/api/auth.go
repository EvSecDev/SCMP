package api

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"scmp/internal/crypto"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/web/internal"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func loginAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	// Artificial wait time for login attempts (post server-side authorization)
	const minProcessingTime = 400 * time.Millisecond
	startTime := time.Now()

	baseCtx = logctx.AppendCtxTag(baseCtx, logctx.NSAuth)

	defer func() {
		// Create random jitter in range
		var jitter time.Duration
		var b [4]byte
		if _, err := rand.Read(b[:]); err == nil {
			n := binary.BigEndian.Uint32(b[:])
			jitterMillis := int64(n%30) - 15
			jitter = time.Duration(jitterMillis) * time.Millisecond
		}

		targetDuration := minProcessingTime + jitter
		elapsed := time.Since(startTime)
		if elapsed < targetDuration {
			time.Sleep(targetDuration - elapsed)
		}
	}()

	req := global.AssertType[UserLogin](fullReq.Params, "req", "UserLogin")

	if !crypto.IsValidUsername(req.Username) {
		logctx.LogStdInfo(baseCtx, "Received invalid login name from user '%s'\n", req.Username)
		errObj.New(rpcUnauthorized, "Unauthorized", "")
		return
	}

	if !crypto.IsValidPassword(req.Password) {
		logctx.LogStdInfo(baseCtx, "Received invalid login password from user '%s'\n", req.Username)
		errObj.New(rpcUnauthorized, "Unauthorized", "")
		return
	}

	// Validate user/pass
	isAuthorized, err := authorizeUser(internal.GetAuthConfig().Users, req.Username, req.Password)
	if err != nil {
		logctx.LogStdInfo(baseCtx, "Encountered error while checking user '%s' authorization\n", req.Username)
		errObj.New(rpcUnauthorized, "Unauthorized", "")
		return
	}
	if !isAuthorized {
		logctx.LogStdInfo(baseCtx, "Received invalid login from user '%s'\n", req.Username)
		errObj.New(rpcUnauthorized, "Unauthorized", "")
		return
	}

	// Generate login token
	now := time.Now().Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":       "https://localhost",
		"sub":       "local-user",
		"name":      req.Username,
		"aud":       []string{"local-client"},
		"iat":       now,
		"exp":       now + int64(internal.GetAuthConfig().JWTValidSec),
		"auth_time": now,
	})

	// Sign the token
	jwtSecret := []byte(internal.GetAuthConfig().JWTSecret)
	jwtString, err := token.SignedString(jwtSecret)
	if err != nil {
		logctx.LogStdInfo(baseCtx, "Failed to sign JWT for user %s: %v\n", req.Username, err)
		errObj.New(rpcUnauthorized, "Unauthorized", "")
		return
	}

	// Return token with redirect
	resp = AuthToken{
		Username:     req.Username,
		Token:        jwtString,
		ValidTime:    internal.GetAuthConfig().JWTValidSec,
		RedirectPage: "/index.html",
	}
	return
}

func authorizeUser(allowedUsers []internal.UserConfig, username string, password string) (isAuthorized bool, err error) {
	// Always validating a hash even when no valid user is found
	const dummyHash = "$argon2id$v=19$m=65536,t=3,p=2$c2FsdGluYm94$Dqz+v7pGp4wHNm1L7E7K6cdCz3Km7F9zqQMd68HcvZM"

	var nameIsValid bool
	matchingUserHash := dummyHash

	// Search all users in config
	for _, allowedUser := range allowedUsers {
		// Skip non-matching names
		if allowedUser.Username != username {
			continue
		}

		// Mark name as valid
		nameIsValid = true
		matchingUserHash = allowedUser.PasswordHash
	}

	// Validate hash against known
	passIsValid, err := crypto.AuthorizeUserPassword(matchingUserHash, password)
	if err != nil {
		err = fmt.Errorf("error validating password: %w", err)
		return
	}

	// Authorized if user+pass is valid
	if nameIsValid && passIsValid {
		isAuthorized = true
	}

	return
}

func VerifyJWT(tokenString string) (validToken *jwt.Token, err error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Make sure the signing method is HMAC (HS256)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenUnverifiable
		}
		return []byte(internal.GetAuthConfig().JWTSecret), nil
	})
	if err != nil {
		err = fmt.Errorf("failed parsing token: %w", err)
		return
	}
	if !token.Valid {
		err = jwt.ErrTokenInvalidClaims
		return
	}
	validToken = token
	return
}

func logoutAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[AuthToken](fullReq.Params, "req", "AuthToken")

	// TODO: Placeholder - implement revocation via database later
	logctx.LogStdInfo(baseCtx, "Logging out user %s\n", req.Username)

	resp = UserLogout{
		RedirectPage: "/login.html",
	}
	return
}

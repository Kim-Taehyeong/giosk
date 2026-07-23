// Package httpx는 핸들러 공용 JSON 응답/에러 헬퍼를 제공한다.
// 핸들러는 이 헬퍼만 사용해 응답 형식을 일관되게 유지한다(로직 금지).
package httpx

import "github.com/gin-gonic/gin"

// OK는 200 + JSON.
func OK(c *gin.Context, body any) { c.JSON(200, body) }

// Created는 201 + JSON.
func Created(c *gin.Context, body any) { c.JSON(201, body) }

// Err는 표준 에러 응답({error, code}).
func Err(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg, "code": code})
}

// BadRequest / Unauthorized / Forbidden / NotFound / Internal — 자주 쓰는 단축.
func BadRequest(c *gin.Context, msg string)   { Err(c, 400, "bad_request", msg) }
func Unauthorized(c *gin.Context, msg string) { Err(c, 401, "unauthorized", msg) }
func Forbidden(c *gin.Context, msg string)    { Err(c, 403, "forbidden", msg) }
func NotFound(c *gin.Context, msg string)     { Err(c, 404, "not_found", msg) }
func Internal(c *gin.Context, msg string)     { Err(c, 500, "internal", msg) }

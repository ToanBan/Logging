package main

import (
	"context"
	"fmt"
	"os"
	"shared/logger"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	dbPool  *pgxpool.Pool
	log     *logger.Logger
	logMode string
)

type ReqLogger interface {
	Info(string, logger.LogContext)
	Warn(string, logger.LogContext)
	Error(string, logger.LogContext)
	Done()
}

type ColumnMeta struct {
	Name   string
	IsID   bool
	IsJSON bool
}

var jsonColumns = map[string]struct{}{
	"metadata": {},
	"payload":  {},
}

func main() {
	logMode = strings.ToLower(strings.TrimSpace(os.Getenv("LOG_MODE")))

	log = logger.NewLogger("gin-service")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5433/logging_benchmark"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Error("failed to parse database URL", logger.LogContext{
			Extra: map[string]any{"error": err.Error()},
		})
		os.Exit(1)
	}

	config.MaxConns = 50

	dbPool, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Error("failed to create database pool", logger.LogContext{
			Extra: map[string]any{"error": err.Error()},
		})
		os.Exit(1)
	}

	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		log.Error("failed to connect to database", logger.LogContext{
			Extra: map[string]any{"error": err.Error()},
		})
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	r.Use(gin.Recovery())

	// request id middleware
	r.Use(func(c *gin.Context) {

		reqID := c.GetHeader("x-request-id")

		if reqID == "" {
			reqID = uuid.New().String()
		}

		c.Header("x-request-id", reqID)
		c.Set("req_id", reqID)

		c.Next()
	})

	r.GET("/health", func(c *gin.Context) {

		if logMode == "structured" {
			log.Info("health check", logger.LogContext{
				ReqID: getReqID(c),
			})
		}

		c.JSON(200, gin.H{
			"ok":      true,
			"service": "gin-service",
		})
	})

	r.GET("/messages", getMessages)
	r.GET("/messages/room/:room_origin_id", getMessagesByRoom)

	port := os.Getenv("PORT")

	if port == "" {
		port = "3003"
	}

	if err := r.Run(":" + port); err != nil {
		log.Fatal("server failed to start", logger.LogContext{
			Extra: map[string]any{"error": err.Error()},
		})
	}
}

// =====================
// helpers
// =====================

func getReqID(c *gin.Context) string {

	if val, exists := c.Get("req_id"); exists {

		if id, ok := val.(string); ok {
			return id
		}
	}

	return ""
}

func getReqLog(reqID string) ReqLogger {

	if logMode == "kafka" {
		return logger.NewRequestLogger("gin-service", reqID)
	}

	return log
}

// =====================
// DB scan helpers
// =====================

func buildColumnMeta(rows pgx.Rows) []ColumnMeta {

	fieldDescriptions := rows.FieldDescriptions()

	metas := make([]ColumnMeta, len(fieldDescriptions))

	for i, fd := range fieldDescriptions {

		name := fd.Name

		_, isJSON := jsonColumns[name]

		metas[i] = ColumnMeta{
			Name: name,
			IsID: name == "id" ||
				name == "room_id_id" ||
				name == "website_room_id_id",
			IsJSON: isJSON,
		}
	}

	return metas
}

func scanRowsToMaps(
	rows pgx.Rows,
	metas []ColumnMeta,
	capacity int,
) ([]map[string]any, error) {

	results := make([]map[string]any, 0, capacity)

	for rows.Next() {

		values, err := rows.Values()

		if err != nil {
			return nil, err
		}

		rowMap := make(map[string]any, len(metas))

		for i, meta := range metas {

			val := values[i]

			if val == nil {
				rowMap[meta.Name] = nil
				continue
			}

			if meta.IsID {

				switch v := val.(type) {

				case string:
					rowMap[meta.Name] = v

				case []byte:
					rowMap[meta.Name] = string(v)

				default:
					rowMap[meta.Name] = fmt.Sprint(v)
				}

				continue
			}

			switch v := val.(type) {

			case time.Time:
				rowMap[meta.Name] =
					v.UTC().Format("2006-01-02T15:04:05.000Z")

			case []byte:

				if meta.IsJSON {

					var jsonVal any

					if err := sonic.Unmarshal(v, &jsonVal); err == nil {
						rowMap[meta.Name] = jsonVal
					} else {
						rowMap[meta.Name] = string(v)
					}

				} else {
					rowMap[meta.Name] = string(v)
				}

			default:
				rowMap[meta.Name] = v
			}
		}

		results = append(results, rowMap)
	}

	return results, nil
}

// =====================
// handlers
// =====================

func getMessages(c *gin.Context) {

	reqID := getReqID(c)
	reqLog := getReqLog(reqID)

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	if logMode == "structured" || logMode == "kafka" {

		reqLog.Info("get messages request", logger.LogContext{
			ReqID: reqID,
			Extra: map[string]any{
				"page":  pageStr,
				"limit": limitStr,
			},
		})
	}

	page, err := strconv.Atoi(pageStr)

	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)

	if err != nil || limit < 1 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	rows, err := dbPool.Query(
		c.Request.Context(),
		`
		SELECT *
		FROM chat_message
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
		`,
		limit,
		offset,
	)

	if err != nil {

		if logMode == "structured" ||
			logMode == "selective" ||
			logMode == "kafka" {

			reqLog.Error("get messages error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{
					"error": err.Error(),
				},
			})
		}

		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})

		return
	}

	defer rows.Close()

	metas := buildColumnMeta(rows)

	messages, err := scanRowsToMaps(rows, metas, limit)

	if err != nil {

		if logMode == "structured" ||
			logMode == "selective" ||
			logMode == "kafka" {

			reqLog.Error("get messages scan error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{
					"error": err.Error(),
				},
			})
		}

		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})

		return
	}

	if logMode == "kafka" {
		reqLog.Done()
	}

	c.JSON(200, gin.H{
		"success": true,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
		"data": messages,
	})
}

func getMessagesByRoom(c *gin.Context) {

	reqID := getReqID(c)
	reqLog := getReqLog(reqID)

	roomOriginID := c.Param("room_origin_id")

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	if logMode == "structured" || logMode == "kafka" {

		reqLog.Info("get messages by room request", logger.LogContext{
			ReqID: reqID,
			Extra: map[string]any{
				"room_origin_id": roomOriginID,
				"page":           pageStr,
				"limit":          limitStr,
			},
		})
	}

	page, err := strconv.Atoi(pageStr)

	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)

	if err != nil || limit < 1 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	rows, err := dbPool.Query(
		c.Request.Context(),
		`
		SELECT *
		FROM chat_message
		WHERE room_origin_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
		`,
		roomOriginID,
		limit,
		offset,
	)

	if err != nil {

		if logMode == "structured" ||
			logMode == "selective" ||
			logMode == "kafka" {

			reqLog.Error("get messages by room error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{
					"room_origin_id": roomOriginID,
					"error":          err.Error(),
				},
			})
		}

		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})

		return
	}

	defer rows.Close()

	metas := buildColumnMeta(rows)

	messages, err := scanRowsToMaps(rows, metas, limit)

	if err != nil {

		if logMode == "structured" ||
			logMode == "selective" ||
			logMode == "kafka" {

			reqLog.Error("get messages by room scan error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{
					"room_origin_id": roomOriginID,
					"error":          err.Error(),
				},
			})
		}

		c.JSON(500, gin.H{
			"success": false,
			"error":   err.Error(),
		})

		return
	}

	if len(messages) == 0 {

		if logMode == "structured" ||
			logMode == "selective" ||
			logMode == "kafka" {

			reqLog.Warn("room not found", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{
					"room_origin_id": roomOriginID,
				},
			})
		}

		c.JSON(404, gin.H{
			"success": false,
			"error": fmt.Sprintf(
				"No messages found for room %s",
				roomOriginID,
			),
		})

		return
	}

	if logMode == "kafka" {
		reqLog.Done()
	}

	c.JSON(200, gin.H{
		"success": true,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
		"data": messages,
	})
}
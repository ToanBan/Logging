package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

var (
	dbPool  *pgxpool.Pool
	logger  zerolog.Logger
	logMode string
)

func main() {
	// Lấy LOG_MODE
	logMode = strings.ToLower(strings.TrimSpace(os.Getenv("LOG_MODE")))
	if logMode == "" {
		logMode = "none"
	}

	// Khởi tạo logger
	if logMode == "none" {
		logger = zerolog.New(io.Discard)
	} else {
		if logMode == "selective" {
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	// Khởi tạo Database Pool
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5433/logging_benchmark"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse database URL")
		os.Exit(1)
	}
	config.MaxConns = 50

	dbPool, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create database pool")
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		logger.Error().Err(err).Msg("Failed to connect to database")
		os.Exit(1)
	}
	logger.Info().Msg("Database connection test successful!")

	// Khởi tạo Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Middleware req_id
	r.Use(func(c *gin.Context) {
		reqID := c.GetHeader("x-request-id")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Header("x-request-id", reqID)

		reqLogger := logger.With().
			Str("req_id", reqID).
			Str("service", "gin-service").
			Logger()
		c.Set("logger", &reqLogger)
		c.Next()
	})

	// Routes
	r.GET("/health", func(c *gin.Context) {
		if logMode == "structured" {
			getLogger(c).Info().Str("event", "health_check").Msg("")
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "service": "gin-service"})
	})

	r.GET("/messages", getMessages)
	r.GET("/messages/room/:room_origin_id", getMessagesByRoom)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3003"
	}
	logger.Info().Msgf("Gin service running on port %s", port)
	if err := r.Run(":" + port); err != nil {
		logger.Fatal().Err(err).Msg("Server failed to start")
	}
}

func getLogger(c *gin.Context) *zerolog.Logger {
	if val, exists := c.Get("logger"); exists {
		if l, ok := val.(*zerolog.Logger); ok {
			return l
		}
	}
	return &logger
}

func scanRowsToMaps(rows pgx.Rows) ([]map[string]interface{}, error) {
	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescriptions))
	for i, fd := range fieldDescriptions {
		columns[i] = fd.Name
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if val != nil {
				if col == "id" || col == "room_id_id" || col == "website_room_id_id" {
					rowMap[col] = fmt.Sprintf("%v", val)
				} else if t, ok := val.(time.Time); ok {
					rowMap[col] = t.UTC().Format("2006-01-02T15:04:05.000Z")
				} else if bytesVal, ok := val.([]byte); ok {
					var jsonVal interface{}
					if err := json.Unmarshal(bytesVal, &jsonVal); err == nil {
						rowMap[col] = jsonVal
					} else {
						rowMap[col] = string(bytesVal)
					}
				} else {
					rowMap[col] = val
				}
			} else {
				rowMap[col] = nil
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func getMessages(c *gin.Context) {
	// ⏱️ BẮT ĐẦU ĐO
	tStart := time.Now()

	reqLog := getLogger(c)
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

	if logMode == "structured" {
		reqLog.Info().Str("event", "get_messages_request").
			Str("page", pageStr).Str("limit", limitStr).Msg("")
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

	// ⏱️ CHẶNG 1: Xong bước tính toán tham số
	tParamDone := time.Now()

	dataQuery := "SELECT * FROM chat_message ORDER BY created_at DESC LIMIT $1 OFFSET $2"
	dataArgs := []interface{}{limit, offset}

	rows, err := dbPool.Query(c.Request.Context(), dataQuery, dataArgs...)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_error").Msg("")
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false, "error": err.Error(),
		})
		return
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_error").Msg("")
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false, "error": err.Error(),
		})
		return
	}

	// ⏱️ CHẶNG 2: Database trả kết quả và Scan xong hoàn toàn
	tDbDone := time.Now()

	// Tính toán thời gian phân đoạn (Chuyển sang dạng mili-giây dạng số thực)
	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6
	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6

	// In kết quả đo đạc ra console giống 2 thằng trước
	fmt.Printf("[GIN_PERF_GET_ALL] Mode: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms\n", strings.ToUpper(logMode), pParams, pDb, pTotal)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"pagination": gin.H{"page": page, "limit": limit},
		"data":       messages,
	})
}

func getMessagesByRoom(c *gin.Context) {
	// ⏱️ BẮT ĐẦU ĐO
	tStart := time.Now()

	reqLog := getLogger(c)
	roomOriginID := c.Param("room_origin_id")

	if logMode == "structured" {
		reqLog.Info().Str("event", "get_messages_by_room_request").Str("room_origin_id", roomOriginID).Msg("")
	}

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")

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

	// ⏱️ CHẶNG 1: Xong bước tính toán tham số
	tParamDone := time.Now()

	rows, err := dbPool.Query(c.Request.Context(), "SELECT * FROM chat_message WHERE room_origin_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3", roomOriginID, limit, offset)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_by_room_error").Str("room_origin_id", roomOriginID).Msg("")
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false, "error": err.Error(),
		})
		return
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_by_room_error").Str("room_origin_id", roomOriginID).Msg("")
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false, "error": err.Error(),
		})
		return
	}

	// ⏱️ CHẶNG 2: Database và Scan hoàn tất
	tDbDone := time.Now()

	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6

	if len(messages) == 0 {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Warn().Str("event", "get_messages_by_room_not_found").Str("room_origin_id", roomOriginID).Msg("")
		}
		
		pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
		// Log lỗi 404 cho Gin
		fmt.Printf("[GIN_PERF_BY_ROOM_404] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms\n", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal)

		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("No messages found for room %s", roomOriginID),
		})
		return
	}

	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
	// Log thành công 200 cho Gin
	fmt.Printf("[GIN_PERF_BY_ROOM_200] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms\n", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"pagination": gin.H{"page": page, "limit": limit},
		"data":       messages,
	})
}
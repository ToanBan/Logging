package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"shared/logger"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	dbPool  *pgxpool.Pool
	log     *logger.Logger
	logMode string
)

func main() {
	logMode = strings.ToLower(strings.TrimSpace(os.Getenv("LOG_MODE")))
	if logMode == "" {
		logMode = "none"
	}

	log = logger.NewLogger("fiber-service")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:5433/logging_benchmark"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Error("failed to parse database URL", logger.LogContext{Extra: map[string]any{"error": err.Error()}})
		os.Exit(1)
	}
	config.MaxConns = 50

	dbPool, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Error("failed to create database pool", logger.LogContext{Extra: map[string]any{"error": err.Error()}})
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(context.Background()); err != nil {
		log.Error("failed to connect to database", logger.LogContext{Extra: map[string]any{"error": err.Error()}})
		os.Exit(1)
	}

	if logMode != "none" {
		log.Info("database connection successful", logger.LogContext{})
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())

	app.Use(func(c *fiber.Ctx) error {
		reqID := c.Get("x-request-id")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Set("x-request-id", reqID)
		c.Locals("req_id", reqID)
		return c.Next()
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		if logMode == "structured" {
			log.Info("health check", logger.LogContext{ReqID: getReqID(c)})
		}
		return c.JSON(fiber.Map{"ok": true, "service": "fiber-service"})
	})

	app.Get("/messages", getMessages)
	app.Get("/messages/room/:room_origin_id", getMessagesByRoom)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3004"
	}

	if logMode != "none" {
		log.Info(fmt.Sprintf("server running on port %s", port), logger.LogContext{})
	}

	if err := app.Listen(":" + port); err != nil {
		log.Fatal("server failed to start", logger.LogContext{Extra: map[string]any{"error": err.Error()}})
	}
}

func getReqID(c *fiber.Ctx) string {
	if val := c.Locals("req_id"); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
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

func getMessages(c *fiber.Ctx) error {
	tStart := time.Now()
	reqID := getReqID(c)

	pageStr := c.Query("page", "1")
	limitStr := c.Query("limit", "10")

	if logMode == "structured" {
		log.Info("get messages request", logger.LogContext{
			ReqID: reqID,
			Extra: map[string]any{"page": pageStr, "limit": limitStr},
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
	tParamDone := time.Now()

	rows, err := dbPool.Query(c.Context(), "SELECT * FROM chat_message ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			log.Error("get messages error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{"error": err.Error()},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			log.Error("get messages scan error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{"error": err.Error()},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	tDbDone := time.Now()
	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6
	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6

	log.Info(fmt.Sprintf("[FIBER_PERF_GET_ALL] Mode: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), pParams, pDb, pTotal), logger.LogContext{ReqID: reqID})

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}

func getMessagesByRoom(c *fiber.Ctx) error {
	tStart := time.Now()
	reqID := getReqID(c)
	roomOriginID := c.Params("room_origin_id")

	if logMode == "structured" {
		log.Info("get messages by room request", logger.LogContext{
			ReqID: reqID,
			Extra: map[string]any{"room_origin_id": roomOriginID},
		})
	}

	pageStr := c.Query("page", "1")
	limitStr := c.Query("limit", "10")

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
	tParamDone := time.Now()

	rows, err := dbPool.Query(c.Context(), "SELECT * FROM chat_message WHERE room_origin_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3", roomOriginID, limit, offset)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			log.Error("get messages by room error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{"room_origin_id": roomOriginID, "error": err.Error()},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			log.Error("get messages by room scan error", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{"room_origin_id": roomOriginID, "error": err.Error()},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	tDbDone := time.Now()
	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6

	if len(messages) == 0 {
		if logMode == "structured" || logMode == "selective" {
			log.Warn("room not found", logger.LogContext{
				ReqID: reqID,
				Extra: map[string]any{"room_origin_id": roomOriginID},
			})
		}

		pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
		log.Info(fmt.Sprintf("[FIBER_PERF_BY_ROOM_404] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal), logger.LogContext{ReqID: reqID})

		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("No messages found for room %s", roomOriginID),
		})
	}

	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
	log.Info(fmt.Sprintf("[FIBER_PERF_BY_ROOM_200] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal), logger.LogContext{ReqID: reqID})

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}

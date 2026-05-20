package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
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
		logger = zerolog.New(io.Discard) // không ghi gì hết
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

	// Khởi tạo Fiber
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())

	// Middleware req_id
	app.Use(func(c *fiber.Ctx) error {
		reqID := c.Get("x-request-id")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Set("x-request-id", reqID)

		reqLogger := logger.With().
			Str("req_id", reqID).
			Str("service", "fiber-service").
			Logger()
		c.Locals("logger", &reqLogger)
		return c.Next()
	})

	// Routes
	app.Get("/health", func(c *fiber.Ctx) error {
		if logMode == "structured" {
			getLogger(c).Info().Str("event", "health_check").Msg("")
		}
		return c.JSON(fiber.Map{"ok": true, "service": "fiber-service"})
	})

	app.Get("/messages", getMessages)
	app.Get("/messages/room/:room_origin_id", getMessagesByRoom)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3004"
	}
	logger.Info().Msgf("Fiber service running on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		logger.Fatal().Err(err).Msg("Server failed to start")
	}
}

func getLogger(c *fiber.Ctx) *zerolog.Logger {
	if val := c.Locals("logger"); val != nil {
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
				// Convert bigint columns to strings to match node-postgres (Fastify) behavior
				if col == "id" || col == "room_id_id" || col == "website_room_id_id" {
					rowMap[col] = fmt.Sprintf("%v", val)
				} else if t, ok := val.(time.Time); ok {
					// Format timestamps as ISO 8601 UTC string (matching JS Date's toISOString())
					rowMap[col] = t.UTC().Format("2006-01-02T15:04:05.000Z")
				} else if bytesVal, ok := val.([]byte); ok {
					// Parse jsonb fields into JSON objects instead of leaving them as raw []byte (base64 string)
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
	reqLog := getLogger(c)
	pageStr := c.Query("page", "1")
	limitStr := c.Query("limit", "10")
	roomOriginID := c.Query("room_origin_id")

	// LOG_MODE switch
	if logMode == "structured" {
		reqLog.Info().Str("event", "get_messages_request").
			Str("page", pageStr).Str("limit", limitStr).Str("room_origin_id", roomOriginID).Msg("")
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

	var dataQuery string
	var dataArgs []interface{}

	if roomOriginID != "" {
		dataQuery = "SELECT * FROM chat_message WHERE room_origin_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3"
		dataArgs = []interface{}{roomOriginID, limit, offset}
	} else {
		dataQuery = "SELECT * FROM chat_message ORDER BY created_at DESC LIMIT $1 OFFSET $2"
		dataArgs = []interface{}{limit, offset}
	}

	rows, err := dbPool.Query(c.Context(), dataQuery, dataArgs...)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_error").Msg("")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false, "error": err.Error(),
		})
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_error").Msg("")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false, "error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}

func getMessagesByRoom(c *fiber.Ctx) error {
	reqLog := getLogger(c)
	roomOriginID := c.Params("room_origin_id")

	if logMode == "structured" {
		reqLog.Info().Str("event", "get_messages_by_room_request").Str("room_origin_id", roomOriginID).Msg("")
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

	rows, err := dbPool.Query(c.Context(), "SELECT * FROM chat_message WHERE room_origin_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3", roomOriginID, limit, offset)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_by_room_error").Str("room_origin_id", roomOriginID).Msg("")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false, "error": err.Error(),
		})
	}
	defer rows.Close()

	messages, err := scanRowsToMaps(rows)
	if err != nil {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Error().Err(err).Str("event", "get_messages_by_room_error").Str("room_origin_id", roomOriginID).Msg("")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false, "error": err.Error(),
		})
	}

	if len(messages) == 0 {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Warn().Str("event", "get_messages_by_room_not_found").Str("room_origin_id", roomOriginID).Msg("")
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("No messages found for room %s", roomOriginID),
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}

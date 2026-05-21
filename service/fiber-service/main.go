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
	logMode = strings.ToLower(strings.TrimSpace(os.Getenv("LOG_MODE")))
	if logMode == "" {
		logMode = "none"
	}

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

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())

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

	reqLog := getLogger(c)
	pageStr := c.Query("page", "1")
	limitStr := c.Query("limit", "10")

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

	tParamDone := time.Now()

	dataQuery := "SELECT * FROM chat_message ORDER BY created_at DESC LIMIT $1 OFFSET $2"
	rows, err := dbPool.Query(c.Context(), dataQuery, limit, offset)
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

	tDbDone := time.Now()

	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6
	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6

	reqLog.Info().
		Str("perf_type", "FIBER_PERF_GET_ALL").
		Str("mode", strings.ToUpper(logMode)).
		Float64("parse_param_ms", pParams).
		Float64("db_query_ms", pDb).
		Float64("total_ms", pTotal).
		Msgf("[FIBER_PERF_GET_ALL] Mode: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), pParams, pDb, pTotal)

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}

func getMessagesByRoom(c *fiber.Ctx) error {
	tStart := time.Now()

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

	tParamDone := time.Now()

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

	tDbDone := time.Now()

	pParams := float64(tParamDone.Sub(tStart).Nanoseconds()) / 1e6
	pDb := float64(tDbDone.Sub(tParamDone).Nanoseconds()) / 1e6

	if len(messages) == 0 {
		if logMode == "structured" || logMode == "selective" {
			reqLog.Warn().Str("event", "get_messages_by_room_not_found").Str("room_origin_id", roomOriginID).Msg("")
		}
		
		pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
		
		reqLog.Info().
			Str("perf_type", "FIBER_PERF_BY_ROOM_404").
			Str("mode", strings.ToUpper(logMode)).
			Str("room_id", roomOriginID).
			Float64("parse_param_ms", pParams).
			Float64("db_query_ms", pDb).
			Float64("total_ms", pTotal).
			Msgf("[FIBER_PERF_BY_ROOM_404] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal)

		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("No messages found for room %s", roomOriginID),
		})
	}

	pTotal := float64(time.Since(tStart).Nanoseconds()) / 1e6
	
	reqLog.Info().
		Str("perf_type", "FIBER_PERF_BY_ROOM_200").
		Str("mode", strings.ToUpper(logMode)).
		Str("room_id", roomOriginID).
		Float64("parse_param_ms", pParams).
		Float64("db_query_ms", pDb).
		Float64("total_ms", pTotal).
		Msgf("[FIBER_PERF_BY_ROOM_200] Mode: %s | Room: %s | ParseParam: %.3fms | DB_Query: %.3fms | Total: %.3fms", strings.ToUpper(logMode), roomOriginID, pParams, pDb, pTotal)

	return c.JSON(fiber.Map{
		"success":    true,
		"pagination": fiber.Map{"page": page, "limit": limit},
		"data":       messages,
	})
}
package indexer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const pythPriceSource = "pyth"

type pythStreamEnvelope struct {
	Parsed []pythPriceUpdate `json:"parsed"`
}

type pythPriceUpdate struct {
	ID       string            `json:"id"`
	Price    pythPriceSnapshot `json:"price"`
	Metadata pythMetadata      `json:"metadata"`
}

type pythPriceSnapshot struct {
	Price       string `json:"price"`
	Conf        string `json:"conf"`
	Expo        int32  `json:"expo"`
	PublishTime int64  `json:"publish_time"`
}

type pythMetadata struct {
	Slot int64 `json:"slot"`
}

func (s *Service) runPythPriceStream(ctx context.Context) {
	if !s.cfg.EnablePythPriceStream {
		return
	}

	endpoint := strings.TrimSpace(s.cfg.PythStreamURL)
	feedID := strings.ToLower(strings.TrimSpace(s.cfg.PythFeedID))
	market := normalizeMarketWithDefault(s.cfg.PythMarket)
	if endpoint == "" || feedID == "" {
		s.logger.Warn("pyth price stream disabled due to missing endpoint or feed id")
		return
	}

	reconnectDelay := s.cfg.PythReconnectInterval
	if reconnectDelay <= 0 {
		reconnectDelay = 3 * time.Second
	}

	client := &http.Client{}
	s.logger.Info(
		"pyth price stream enabled",
		"endpoint", endpoint,
		"feed_id", feedID,
		"market", market,
		"reconnect_delay", reconnectDelay.String(),
	)

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		err := s.consumePythPriceStream(ctx, client, endpoint, feedID, market)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Warn("pyth price stream disconnected", "err", err, "retry_in", reconnectDelay.String())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (s *Service) consumePythPriceStream(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	feedID string,
	market string,
) error {
	streamURL, err := buildPythStreamURL(endpoint, feedID)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return fmt.Errorf("build pyth stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("open pyth stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("open pyth stream: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024), 64*1024*1024)

	var eventData strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if eventData.Len() == 0 {
				continue
			}
			if err := s.processPythStreamEvent(ctx, eventData.String(), market, feedID); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Warn("failed to process pyth stream event", "err", err)
			}
			eventData.Reset()
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if eventData.Len() > 0 {
			eventData.WriteByte('\n')
		}
		eventData.WriteString(payload)
	}

	if eventData.Len() > 0 {
		if err := s.processPythStreamEvent(ctx, eventData.String(), market, feedID); err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Warn("failed to process final pyth stream event", "err", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read pyth stream: %w", err)
	}

	return io.EOF
}

func (s *Service) processPythStreamEvent(ctx context.Context, rawEvent string, market string, feedID string) error {
	payload := strings.TrimSpace(rawEvent)
	if payload == "" || payload == "[DONE]" {
		return nil
	}

	var event pythStreamEnvelope
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("decode pyth stream event: %w", err)
	}
	if len(event.Parsed) == 0 {
		return nil
	}

	now := time.Now().Unix()
	for _, update := range event.Parsed {
		updateID := strings.ToLower(strings.TrimSpace(update.ID))
		if updateID == "" {
			continue
		}
		if updateID != feedID {
			continue
		}

		price, err := decodePythPrice(update.Price.Price, update.Price.Expo)
		if err != nil || price <= 0 {
			continue
		}
		conf, err := decodePythPrice(update.Price.Conf, update.Price.Expo)
		if err != nil {
			conf = 0
		}

		publishTime := update.Price.PublishTime
		if publishTime <= 0 {
			publishTime = now
		}

		rawUpdate, err := json.Marshal(update)
		if err != nil {
			rawUpdate = []byte("{}")
		}

		_, err = s.store.InsertMarketPriceTick(ctx, MarketPriceTickInput{
			Market:      market,
			Source:      pythPriceSource,
			FeedID:      updateID,
			Slot:        update.Metadata.Slot,
			PublishTime: publishTime,
			Price:       price,
			Conf:        conf,
			Expo:        update.Price.Expo,
			ReceivedAt:  now,
			RawJSON:     string(rawUpdate),
		})
		if err != nil {
			return fmt.Errorf("store pyth tick: %w", err)
		}
	}

	return nil
}

func buildPythStreamURL(endpoint string, feedID string) (string, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("parse pyth endpoint: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("invalid pyth endpoint: %q", endpoint)
	}

	query := parsedURL.Query()
	query.Del("ids[]")
	query.Add("ids[]", feedID)
	if strings.TrimSpace(query.Get("parsed")) == "" {
		query.Set("parsed", "true")
	}
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func decodePythPrice(raw string, expo int32) (float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("empty price")
	}

	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, err
	}

	if expo < 0 {
		return value / math.Pow10(int(-expo)), nil
	}
	if expo > 0 {
		return value * math.Pow10(int(expo)), nil
	}
	return value, nil
}

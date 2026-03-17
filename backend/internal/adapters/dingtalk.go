package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DingTalkAdapter struct {
	webhookURL string
}

func NewDingTalkAdapter(webhookURL string) *DingTalkAdapter {
	return &DingTalkAdapter{
		webhookURL: webhookURL,
	}
}

func (d *DingTalkAdapter) GetID() string {
	return "dingtalk"
}

type dingTalkPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"markdown"`
}

func (d *DingTalkAdapter) SendSummary(ctx context.Context, summary string) error {
	payload := dingTalkPayload{
		MsgType: "markdown",
	}
	payload.Markdown.Title = "Message Summary"
	payload.Markdown.Text = fmt.Sprintf("### Message Summary (%s)\n\n%s", time.Now().Format("2006-01-02 15:04"), summary)

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk returned status code %d", resp.StatusCode)
	}

	return nil
}

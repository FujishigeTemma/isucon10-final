package xsuportal

import (
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang/protobuf/proto"
	"github.com/jmoiron/sqlx"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/isucon/isucon10-final/webapp/golang/proto/xsuportal/resources"
)

const (
	WebpushVAPIDPrivateKeyPath = "../vapid_private.pem"
	WebpushSubject             = "xsuportal@example.com"
)

type Notifier struct {
	mu      sync.Mutex
	options *webpush.Options
}

func (n *Notifier) VAPIDKey() *webpush.Options {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.options == nil {
		pemBytes, err := ioutil.ReadFile(WebpushVAPIDPrivateKeyPath)
		if err != nil {
			fmt.Println("read file error")
			fmt.Println(err)
			return nil
		}
		block, _ := pem.Decode(pemBytes)
		if block == nil {
			return nil
		}
		priKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil
		}
		priBytes := priKey.D.Bytes()
		pubBytes := elliptic.Marshal(priKey.Curve, priKey.X, priKey.Y)
		pri := base64.RawURLEncoding.EncodeToString(priBytes)
		pub := base64.RawURLEncoding.EncodeToString(pubBytes)
		n.options = &webpush.Options{
			Subscriber:      WebpushSubject,
			VAPIDPrivateKey: pri,
			VAPIDPublicKey:  pub,
		}
	}
	return n.options
}

func SendWebPush(vapidPrivateKey, vapidPublicKey string, notificationPB *resources.Notification, pushSubscription *PushSubscription) error {
	b, err := proto.Marshal(notificationPB)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	message := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
	base64.StdEncoding.Encode(message, b)

	resp, err := webpush.SendNotification(
		message,
		&webpush.Subscription{
			Endpoint: pushSubscription.Endpoint,
			Keys: webpush.Keys{
				Auth:   pushSubscription.Auth,
				P256dh: pushSubscription.P256DH,
			},
		},
		&webpush.Options{
			Subscriber:      WebpushSubject,
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
		},
	)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()
	expired := resp.StatusCode == 410
	if expired {
		fmt.Println(err)
		return fmt.Errorf("expired notification")
	}
	invalid := resp.StatusCode == 404
	if invalid {
		fmt.Println(err)
		return fmt.Errorf("invalid notification")
	}
	return nil
}

func getTargetsMapFromIDs(db sqlx.Ext, ids []string) (map[string]PushSubscription, error) {
	inQuery, inArgs, err := sqlx.In("SELECT * FROM `push_subscriptions` WHERE `contestant_id` IN (?)", ids)
	if err != nil {
		fmt.Println("error in constructing query in getTargetsFromIDs")
		fmt.Printf("%#v", err)
		return nil, err
	}
	targetInfos := []PushSubscription{}
	err = sqlx.Select(
		db,
		&targetInfos,
		inQuery, inArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("select all contestants: %w", err)
	}
	res := map[string]PushSubscription{}
	for _, t := range targetInfos {
		res[t.ContestantID] = t
	}
	return res, nil
}

func (n *Notifier) NotifyClarificationAnswered(db sqlx.Ext, c *Clarification, updated bool) error {
	var contestants []struct {
		ID     string `db:"id"`
		TeamID int64  `db:"team_id"`
	}
	if c.Disclosed.Valid && c.Disclosed.Bool {
		err := sqlx.Select(
			db,
			&contestants,
			"SELECT `id`, `team_id` FROM `contestants` WHERE `team_id` IS NOT NULL",
		)
		if err != nil {
			return fmt.Errorf("select all contestants: %w", err)
		}
	} else {
		err := sqlx.Select(
			db,
			&contestants,
			"SELECT `id`, `team_id` FROM `contestants` WHERE `team_id` = ?",
			c.TeamID,
		)
		if err != nil {
			fmt.Printf("select contestants(team_id=%v): %#v\n", c.TeamID, err) 
			return fmt.Errorf("select contestants(team_id=%v): %w", c.TeamID, err)
		}
	}

	ids := []string{}
	for _, c := range contestants {
		ids = append(ids, c.ID)
	}
	// TODO: JOINでとれる
	infoMap, err := getTargetsMapFromIDs(db, ids)
	if err != nil {
		return err
	}
	if !c.Disclosed.Valid || !c.Disclosed.Bool {
		fmt.Println("this request has targeted team")
		fmt.Printf("ids: %#v\n", ids)
		fmt.Printf("map: %#v\n", infoMap)
	}
	for _, contestant := range contestants {
		notificationPB := &resources.Notification{
			Content: &resources.Notification_ContentClarification{
				ContentClarification: &resources.Notification_ClarificationMessage{
					ClarificationId: c.ID,
					Owned:           c.TeamID == contestant.TeamID,
					Updated:         updated,
				},
			},
		}
		notification, err := n.notify(db, notificationPB, contestant.ID)
		if err != nil {
			return fmt.Errorf("notify: %w", err)
		}
		if n.options != nil {
			notificationPB.Id = notification.ID
			notificationPB.CreatedAt = timestamppb.New(notification.CreatedAt)
			info, exist := infoMap[contestant.ID]
			if !exist {
				fmt.Println("exist not subscribe user")
				return fmt.Errorf("not subscribe")
			}
			err = SendWebPush(n.options.VAPIDPrivateKey, n.options.VAPIDPublicKey, notificationPB, &info)
			if err != nil {
				fmt.Printf("is to team: %#v", !c.Disclosed.Valid || !c.Disclosed.Bool)
				fmt.Printf("err in sendwebpush: %#v", err)
			}
		}
	}
	return nil
}

func (n *Notifier) NotifyBenchmarkJobFinished(db sqlx.Ext, job *BenchmarkJob) error {
	fmt.Println("start webpush (bench)")
	var contestants []struct {
		ID     string `db:"id"`
		TeamID int64  `db:"team_id"`
	}
	err := sqlx.Select(
		db,
		&contestants,
		"SELECT `id`, `team_id` FROM `contestants` WHERE `team_id` = ?",
		job.TeamID,
	)
	if err != nil {
		fmt.Printf("select contestants(team_id=%v): %#v\n", job.TeamID, err)
		return fmt.Errorf("select contestants(team_id=%v): %w", job.TeamID, err)
	}
	ids := []string{}
	for _, c := range contestants {
		ids = append(ids, c.ID)
	}
	// TODO: JOINでとれる
	infoMap, err := getTargetsMapFromIDs(db, ids)
	if err != nil {
		fmt.Println("error in getTargetsMapFromIDs")
		fmt.Println(err)
		return err
	}
	for _, contestant := range contestants {
		notificationPB := &resources.Notification{
			Content: &resources.Notification_ContentBenchmarkJob{
				ContentBenchmarkJob: &resources.Notification_BenchmarkJobMessage{
					BenchmarkJobId: job.ID,
				},
			},
		}
		notification, err := n.notify(db, notificationPB, contestant.ID)
		if err != nil {
			fmt.Println(err)
			return fmt.Errorf("notify: %w", err)
		}
		if n.options != nil {
			notificationPB.Id = notification.ID
			notificationPB.CreatedAt = timestamppb.New(notification.CreatedAt)
			info, exist := infoMap[contestant.ID]
			if !exist {
				fmt.Println("exist not subscribe user")
				return fmt.Errorf("not subscribe")
			}
			SendWebPush(n.options.VAPIDPrivateKey, n.options.VAPIDPublicKey, notificationPB, &info)
		}
	}
	return nil
}

func (n *Notifier) notify(db sqlx.Ext, notificationPB *resources.Notification, contestantID string) (*Notification, error) {
	m, err := proto.Marshal(notificationPB)
	if err != nil {
		return nil, fmt.Errorf("marshal notification: %w", err)
	}
	encodedMessage := base64.StdEncoding.EncodeToString(m)
	res, err := db.Exec(
		"INSERT INTO `notifications` (`contestant_id`, `encoded_message`, `read`, `created_at`, `updated_at`) VALUES (?, ?, FALSE, NOW(6), NOW(6))",
		contestantID,
		encodedMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	lastInsertID, _ := res.LastInsertId()
	// notification := Notification{}
	var notification Notification
	err = sqlx.Get(
		db,
		&notification,
		"SELECT * FROM `notifications` WHERE `id` = ? LIMIT 1",
		lastInsertID,
	)
	if err != nil {
		return nil, fmt.Errorf("get inserted notification: %w", err)
	}
	return &notification, nil
}

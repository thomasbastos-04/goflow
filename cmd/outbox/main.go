package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/thomas-dev7/goflow/internal/config"
	"github.com/thomas-dev7/goflow/internal/platform"
)

func main() {
	log,_:=zap.NewProduction()
	defer log.Sync()
	cfg,err:=config.Load()
	if err!=nil { log.Fatal("config",zap.Error(err)) }

	ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM)
	defer cancel()

	db,err:=platform.OpenPostgres(ctx,cfg.DatabaseURL)
	if err!=nil { log.Fatal("postgres",zap.Error(err)) }
	defer db.Close()

	conn,err:=platform.OpenRabbit(cfg.RabbitMQURL)
	if err!=nil { log.Fatal("rabbitmq",zap.Error(err)) }
	defer conn.Close()
	ch,err:=conn.Channel()
	if err!=nil { log.Fatal("channel",zap.Error(err)) }
	defer ch.Close()
	if err:=platform.DeclareRabbit(ch); err!=nil { log.Fatal("declare",zap.Error(err)) }
	if err:=ch.Confirm(false); err!=nil { log.Fatal("confirm",zap.Error(err)) }
	confirms:=ch.NotifyPublish(make(chan amqp.Confirmation,1))

	ticker:=time.NewTicker(time.Second)
	defer ticker.Stop()
	log.Info("outbox_started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows,err:=db.Query(ctx,`
				SELECT id,event_type,payload::text
				FROM outbox_events
				WHERE published_at IS NULL
				ORDER BY created_at
				LIMIT 50`)
			if err!=nil { log.Error("outbox_query",zap.Error(err)); continue }

			type evt struct{id,typ,payload string}
			var events []evt
			for rows.Next() {
				var e evt
				if err:=rows.Scan(&e.id,&e.typ,&e.payload); err==nil { events=append(events,e) }
			}
			rows.Close()

			for _,e:=range events {
				err=ch.PublishWithContext(ctx,platform.OrderExchange,e.typ,false,false,amqp.Publishing{
					ContentType:"application/json",
					DeliveryMode:amqp.Persistent,
					MessageId:e.id,
					Timestamp:time.Now(),
					Body:[]byte(e.payload),
				})
				if err!=nil { log.Error("publish",zap.Error(err)); break }
				confirm:=<-confirms
				if !confirm.Ack { log.Error("publish_nack",zap.String("event_id",e.id)); break }
				if _,err:=db.Exec(ctx,`UPDATE outbox_events SET published_at=NOW() WHERE id=$1`,e.id); err!=nil {
					log.Error("mark_published",zap.Error(err))
					break
				}
				log.Info("event_published",zap.String("event_id",e.id),zap.String("type",e.typ))
			}
		}
	}
}

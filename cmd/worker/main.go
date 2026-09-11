package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/thomasbastos-04/goflow/internal/config"
	"github.com/thomasbastos-04/goflow/internal/domain"
	"github.com/thomasbastos-04/goflow/internal/platform"
	"github.com/thomasbastos-04/goflow/internal/repository"
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
	repo:=repository.New(db)

	conn,err:=platform.OpenRabbit(cfg.RabbitMQURL)
	if err!=nil { log.Fatal("rabbitmq",zap.Error(err)) }
	defer conn.Close()
	ch,err:=conn.Channel()
	if err!=nil { log.Fatal("channel",zap.Error(err)) }
	defer ch.Close()
	if err:=platform.DeclareRabbit(ch); err!=nil { log.Fatal("declare",zap.Error(err)) }
	_ = ch.Qos(10,0,false)

	msgs,err:=ch.Consume(platform.OrderQueue,"",false,false,false,false,nil)
	if err!=nil { log.Fatal("consume",zap.Error(err)) }

	log.Info("worker_started")
	for {
		select {
		case <-ctx.Done():
			return
		case msg,ok:=<-msgs:
			if !ok { return }
			var event domain.OrderCreatedEvent
			if err:=json.Unmarshal(msg.Body,&event); err!=nil {
				log.Error("invalid_event",zap.Error(err))
				_ = msg.Nack(false,false)
				continue
			}
			// Simulated external payment latency.
			time.Sleep(250*time.Millisecond)
			status:=domain.OrderPaid
			if event.TotalCents%13==0 { status=domain.OrderFailed }

			if err:=repo.UpdateOrderStatus(ctx,event.OrderID,status); err!=nil {
				log.Error("process_order",zap.Error(err),zap.String("order_id",event.OrderID))
				_ = msg.Nack(false,false)
				continue
			}
			log.Info("order_processed",zap.String("order_id",event.OrderID),zap.String("status",status))
			_ = msg.Ack(false)
		}
	}
}

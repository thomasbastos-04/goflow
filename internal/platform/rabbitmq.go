package platform

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	OrderExchange = "orders.events"
	OrderQueue    = "orders.created"
	OrderDLX      = "orders.dlx"
	OrderDLQ      = "orders.dlq"
)

func OpenRabbit(url string) (*amqp.Connection, error) {
	var conn *amqp.Connection
	var err error
	for i := 0; i < 10; i++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("rabbitmq connection: %w", err)
}

func DeclareRabbit(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(OrderExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(OrderDLX, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	args := amqp.Table{
		"x-dead-letter-exchange":    OrderDLX,
		"x-dead-letter-routing-key": "dead",
	}
	if _, err := ch.QueueDeclare(OrderQueue, true, false, false, false, args); err != nil {
		return err
	}
	if err := ch.QueueBind(OrderQueue, "order.created", OrderExchange, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(OrderDLQ, true, false, false, false, nil); err != nil {
		return err
	}
	return ch.QueueBind(OrderDLQ, "dead", OrderDLX, false, nil)
}

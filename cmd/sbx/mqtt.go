package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"

	"github.com/brunoga/sofabaton/hub"
)

// mqttPasswordEnv names the variable that can give the broker's password,
// to keep it out of the process list.
const mqttPasswordEnv = "SBX_MQTT_PASSWORD" //nolint:gosec // the variable's name, not a credential

// mqttCmd shows whether the X2 has an MQTT broker, sets one, or unbinds it.
func mqttCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("mqtt", flag.ContinueOnError)
	var h hubFlags
	h.register(fs)
	broker := fs.String("broker", "", "the broker to set, IPv4:port (e.g. 192.168.1.10:1883)")
	user := fs.String("user", "", "the broker's user (required with -broker)")
	password := fs.String("password", "", "the broker's password (or $"+mqttPasswordEnv+")")
	clear := fs.Bool("clear", false, "unbind the hub's broker")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *broker != "" && *clear {
		return errors.New("-broker and -clear go separately")
	}
	var b hub.MQTTBroker
	if *broker != "" {
		ap, err := netip.ParseAddrPort(*broker)
		if err != nil {
			return fmt.Errorf("-broker: %w", err)
		}
		if *password == "" {
			*password = os.Getenv(mqttPasswordEnv)
		}
		if *user == "" || *password == "" {
			return errors.New("-broker needs -user and -password (the hub takes no broker without them)")
		}
		b = hub.MQTTBroker{Addr: ap, User: *user, Password: *password}
	}
	c, err := h.connect(ctx, stderr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	switch {
	case *clear:
		if err := c.ClearMQTT(ctx); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "unbound the hub's MQTT broker")
		return nil
	case *broker != "":
		if err := c.SetMQTT(ctx, b); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "set the hub's MQTT broker to %v\n", b.Addr)
	}
	ok, err := c.MQTTConfigured(ctx)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(stdout, "the hub has no MQTT broker")
		return nil
	}
	mac := strings.ToUpper(fmt.Sprintf("%x", c.Info().ID[:6]))
	fmt.Fprintf(stdout, "the hub has an MQTT broker; it publishes the MQTT commands it runs to %s/up\n", mac)
	fmt.Fprintf(stdout, "and answers requests on activity/%s/... and device/%s/... (docs/protocol/mqtt.md)\n", mac, mac)
	return nil
}

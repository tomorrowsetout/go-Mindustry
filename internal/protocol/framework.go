package protocol

// Framework message IDs used by ArcNet.
const (
	FrameworkPing       = 0
	FrameworkDiscover   = 1
	FrameworkKeepAlive  = 2
	FrameworkRegisterUD = 3
	FrameworkRegisterTC = 4
)

type FrameworkMessage interface {
	FrameworkID() byte
}

type Ping struct {
	ID      int32
	IsReply bool
}

func (p *Ping) FrameworkID() byte { return FrameworkPing }

type DiscoverHost struct{}

func (d *DiscoverHost) FrameworkID() byte { return FrameworkDiscover }

type KeepAlive struct{}

func (k *KeepAlive) FrameworkID() byte { return FrameworkKeepAlive }

type RegisterUDP struct {
	ConnectionID int32
}

func (r *RegisterUDP) FrameworkID() byte { return FrameworkRegisterUD }

type RegisterTCP struct {
	ConnectionID int32
}

func (r *RegisterTCP) FrameworkID() byte { return FrameworkRegisterTC }

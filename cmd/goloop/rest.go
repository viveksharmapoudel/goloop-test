package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/icon-project/goloop/module"
	"github.com/icon-project/goloop/network"
	"github.com/icon-project/goloop/server/metric"
)

const (
	UrlSystem   = "/system"
	UrlChain    = "/chain"
	ParamNID    = "nid"
	UrlChainRes = "/:" + ParamNID
)

type Rest struct {
	n *Node
}

type SystemView struct {
	Address       string `json:"address"`
	P2PAddr       string `json:"p2p"`
	P2PListenAddr string `json:"p2p_listen"`
}

type JoinChainParam struct {
	NID    int    `json:"nid"`
	DBType string `json:"db_type"`

	SeedAddr         string `json:"seed_addr"`
	Role             uint   `json:"role"`
	ConcurrencyLevel int    `json:"concurrency_level,omitempty"`

	Channel string `json:"channel"`

	Genesis json.RawMessage `json:"genesis"`
}

type ChainView struct {
	NID       int    `json:"nid"`
	State     string `json:"state"`
	Height    int64  `json:"height"`
	LastError string `json:"lasterr"`
}

type ChainInspectView struct {
	*ChainView
	Genesis json.RawMessage        `json:"genesis"`
	Module  map[string]interface{} `json:"module"`
}

//TODO [TBD]move to module.Chain ?
type LastErrorReportor interface {
	LastError() error
}

func NewChainView(c module.Chain) *ChainView {
	v := &ChainView{
		NID:   c.NID(),
		State: c.State(),
	}
	if r, ok := c.(LastErrorReportor); ok && r.LastError() != nil {
		v.LastError = r.LastError().Error()
	}

	if bm := c.BlockManager(); bm != nil {
		if b, err := bm.GetLastBlock(); err == nil {
			v.Height = b.Height()
		}
	}
	return v
}

type InspectFunc func(c module.Chain) map[string]interface{}

var (
	inspectFuncs = make(map[string]InspectFunc)
)

func NewChainInspectView(c module.Chain) *ChainInspectView {
	v := &ChainInspectView{
		ChainView: NewChainView(c),
		Genesis:   c.Genesis(),
	}
	v.Module = make(map[string]interface{})
	for name, f := range inspectFuncs {
		v.Module[name] = f(c)
	}
	return v
}

func RegisterInspectFunc(name string, f InspectFunc) error {
	if _, ok := inspectFuncs[name]; ok {
		return fmt.Errorf("already exist function name:%s", name)
	}
	inspectFuncs[name] = f
	return nil
}

func RegisterRest(n *Node) {
	r := Rest{n}
	ag := n.srv.AdminEchoGroup()
	r.RegisterChainHandlers(ag.Group(UrlChain))
	r.RegisterSystemHandlers(ag.Group(UrlSystem))

	r.RegisterChainHandlers(n.cliSrv.e.Group(UrlChain))
	r.RegisterSystemHandlers(n.cliSrv.e.Group(UrlSystem))

	_ = RegisterInspectFunc("metrics", metric.Inspect)
	_ = RegisterInspectFunc("network", network.Inspect)
}

func (r *Rest) RegisterChainHandlers(g *echo.Group) {
	g.GET("", r.GetChains)
	g.POST("", r.JoinChain)

	g.GET(UrlChainRes, r.GetChain, r.ChainInjector)
	g.DELETE(UrlChainRes, r.GetChain, r.ChainInjector)
	g.DELETE(UrlChainRes, r.LeaveChain, r.ChainInjector)
	//TODO update chain configuration ex> Channel, Seed, ConcurrencyLevel ...
	//g.PUT(UrlChainRes, r.UpdateChain, r.ChainInjector)
	g.POST(UrlChainRes+"/start", r.StartChain, r.ChainInjector)
	g.POST(UrlChainRes+"/stop", r.StopChain, r.ChainInjector)
	g.POST(UrlChainRes+"/reset", r.ResetChain, r.ChainInjector)
	g.POST(UrlChainRes+"/verify", r.VerifyChain, r.ChainInjector)
}

func (r *Rest) ChainInjector(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		NID, err := strconv.ParseInt(ctx.Param(ParamNID), 16, 64)
		if err != nil {
			return err
		}
		c := r.n.GetChain(int(NID))
		if c == nil {
			return ctx.NoContent(http.StatusNotFound)
		}
		ctx.Set("chain", c)
		return next(ctx)
	}
}

func (r *Rest) GetChains(ctx echo.Context) error {
	l := make([]*ChainView, 0)
	for _, c := range r.n.GetChains() {
		v := NewChainView(c)
		l = append(l, v)
	}
	return ctx.JSON(http.StatusOK, l)
}

func GetJsonMultipart(ctx echo.Context, ptr interface{}) error {
	ff, err := ctx.FormFile("json")
	if err != nil {
		return err
	}
	f, err := ff.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, ptr); err != nil {
		return err
	}
	return nil
}

func GetFileMultipart(ctx echo.Context, fieldname string) ([]byte, error) {
	ff, err := ctx.FormFile(fieldname)
	if err != nil {
		return nil, err
	}
	f, err := ff.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *Rest) JoinChain(ctx echo.Context) error {
	var err error
	p := &JoinChainParam{}
	//if err = ctx.Bind(p); err != nil {
	//	log.Println("Warning", err)
	//	return err
	//}
	if err = GetJsonMultipart(ctx, p); err != nil {
		log.Println("Warning", err)
		return err
	}

	if c := r.n.GetChain(p.NID); c != nil {
		return ctx.NoContent(http.StatusConflict)
	}

	genesis, err := GetFileMultipart(ctx, "genesisZip")
	if err != nil {
		log.Println("Warning", err)
		return err
	}

	//gs, err := chain.NewGenesisStorage(b)
	//
	//gs, err := chain.NewGenesisStorageWithDataDir(p.Genesis,"")
	//if err != nil {
	//	log.Println("Warning", err)
	//	return err
	//}
	_, err = r.n.JoinChain(p.NID, p.SeedAddr, p.Role, p.DBType, p.ConcurrencyLevel, genesis)
	if err != nil {
		log.Println("Warning", err)
		return err
	}
	return ctx.JSON(http.StatusOK, "OK")
}

func (r *Rest) GetChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	return ctx.JSON(http.StatusOK, NewChainInspectView(c))
}

func (r *Rest) LeaveChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	if err := r.n.LeaveChain(c.NID()); err != nil {
		return err
	}
	return ctx.String(http.StatusOK, "OK")
}

func (r *Rest) StartChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	if err := r.n.StartChain(c.NID()); err != nil {
		return err
	}
	return ctx.String(http.StatusOK, "OK")
}

func (r *Rest) StopChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	if err := r.n.StopChain(c.NID()); err != nil {
		return err
	}
	return ctx.String(http.StatusOK, "OK")
}

func (r *Rest) ResetChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	if err := r.n.ResetChain(c.NID()); err != nil {
		return err
	}
	return ctx.String(http.StatusOK, "OK")
}

func (r *Rest) VerifyChain(ctx echo.Context) error {
	c := ctx.Get("chain").(module.Chain)
	if err := r.n.VerifyChain(c.NID()); err != nil {
		return err
	}
	return ctx.String(http.StatusOK, "OK")
}

func (r *Rest) RegisterSystemHandlers(g *echo.Group) {
	g.GET("", r.GetSystem)
}

func (r *Rest) GetSystem(ctx echo.Context) error {
	v := &SystemView{
		Address:       r.n.w.Address().String(),
		P2PAddr:       r.n.nt.Address(),
		P2PListenAddr: r.n.nt.GetListenAddress(),
	}
	return ctx.JSON(http.StatusOK, v)
}
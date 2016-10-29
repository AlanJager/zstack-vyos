package plugin

import (
	"zvr/server"
	"zvr/utils"
	"fmt"
)

const (
	SET_SNAT_PATH = "/setsnat"
	REMOVE_SNAT_PATH = "/removesnat"
	SYNC_SNAT_PATH = "/syncsnat"
)

type snatInfo struct {
	PublicNicMac string `json:"publicNicMac"`
	PublicIp string `json:"publicIp"`
	PrivateNicMac string `json:"privateNicMac"`
	PrivateNicIp string `json:"privateNicIp"`
	SnatNetmask string `json:"snatNetmask"`
}

type setSnatCmd struct {
	Snat snatInfo `json:"snat"`
}

type removeSnatCmd struct {
	natInfo []snatInfo `json:"natInfo"`
}

type syncSnatCmd struct {
	snats []snatInfo `json:"snats"`
}

func setSnatHandler(ctx *server.CommandContext) interface{} {
	cmd := &setSnatCmd{}
	ctx.GetCommand(cmd)

	s := cmd.Snat
	tree := server.NewParserFromShowConfiguration().Tree
	outNic, err := utils.GetNicNameByMac(s.PublicNicMac); utils.PanicOnError(err)
	subnetNumber, err := utils.GetNetworkNumber(s.PrivateNicIp, s.SnatNetmask); utils.PanicOnError(err)
	cidr, err := utils.NetmaskToCIDR(s.SnatNetmask); utils.PanicOnError(err)
	address := fmt.Sprintf("%v/%v", subnetNumber, cidr)

	if hasRuleNumberForAddress(tree, address) {
		panic(fmt.Errorf("there has been a source nat for the network[%s]", address))
	}

	tree.SetSnat(
		fmt.Sprintf("outbound-interface %s", outNic),
		fmt.Sprintf("source address %v", address),
		"translation address masquerade",
	)

	tree.Apply(false)

	return nil
}

func removeSnatHandler(ctx *server.CommandContext) interface{} {
	cmd := &removeSnatCmd{}
	ctx.GetCommand(&cmd)

	tree := server.NewParserFromShowConfiguration().Tree
	rs := tree.Get("nat source rule")
	if rs == nil {
		return nil
	}

	for _, s := range cmd.natInfo {
		subnetNumber, err := utils.GetNetworkNumber(s.PrivateNicIp, s.SnatNetmask); utils.PanicOnError(err)
		cidr, err := utils.NetmaskToCIDR(s.SnatNetmask); utils.PanicOnError(err)
		address := fmt.Sprintf("%v/%v", subnetNumber, cidr)

		for _, r := range rs.Children() {
			if addr := r.Get("source address"); addr != nil && addr.Value() == address {
				addr.Delete()
			}
		}
	}

	tree.Apply(false)

	return nil
}

func hasRuleNumberForAddress(tree *server.VyosConfigTree, address string) bool {
	rs := tree.Get("nat source rule")
	if rs == nil {
		return false
	}

	for _, r := range rs.Children() {
		if addr := r.Get("source address"); addr != nil && addr.Value() == address {
			return true
		}
	}

	return false
}

func syncSnatHandler(ctx *server.CommandContext) interface{} {
	cmd := &syncSnatCmd{}
	ctx.GetCommand(cmd)

	tree := server.NewParserFromShowConfiguration().Tree
	for _, s := range cmd.snats {
		outNic, err := utils.GetNicNameByMac(s.PublicNicMac); utils.PanicOnError(err)
		subnetNumber, err := utils.GetNetworkNumber(s.PrivateNicIp, s.SnatNetmask); utils.PanicOnError(err)
		cidr, err := utils.NetmaskToCIDR(s.SnatNetmask); utils.PanicOnError(err)
		address := fmt.Sprintf("%v/%v", subnetNumber, cidr)

		if !hasRuleNumberForAddress(tree, address) {
			tree.SetSnat(
				fmt.Sprintf("outbound-interface %s", outNic),
				fmt.Sprintf("source address %s", address),
				"translation address masquerade",
			)
		}
	}

	tree.Apply(false)

	return nil
}

func SnatEntryPoint() {
	server.RegisterAsyncCommandHandler(SET_SNAT_PATH, server.VyosLock(setSnatHandler))
	server.RegisterAsyncCommandHandler(REMOVE_SNAT_PATH, server.VyosLock(removeSnatHandler))
	server.RegisterAsyncCommandHandler(SYNC_SNAT_PATH, server.VyosLock(syncSnatHandler))
}
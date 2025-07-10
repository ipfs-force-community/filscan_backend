package upgrader

//var NetworkActors = &networkActors{}
//
//type networkActors struct {
//	lock              sync.Mutex
//	maxNetworkVersion int64 // 已记录的最大网络版本
//	networkActorTypes map[int64]map[string]string
//}
//
//type NetworkActorsNode interface {
//	StateActorCodeCIDs(p0 context.Context, p1 abinetwork.Version) (map[string]cid.Cid, error)
//	StateNetworkVersion(p0 context.Context, p1 types.TipSetKey) (apitypes.NetworkVersion, error)
//}
//
//// Register
//// ‼️ 请注意：Infra 是阉割版本，没有 StateActorCodeCIDs 方法
//func (n *networkActors) Register(node NetworkActorsNode) (err error) {
//	n.lock.Lock()
//	defer func() {
//		n.lock.Unlock()
//	}()
//
//	if n.networkActorTypes == nil {
//		n.networkActorTypes = map[int64]map[string]string{}
//	}
//
//	nv, err := node.StateNetworkVersion(context.Background(), types.EmptyTSK)
//	if err != nil {
//		err = fmt.Errorf("register network actor types error: %s", err)
//		return
//	}
//
//	if int64(nv) <= n.maxNetworkVersion {
//		return
//	}
//
//	for network := int64(nv); network >= 0; network-- {
//		if _, ok := n.networkActorTypes[network]; ok {
//			continue
//		}
//		var codes map[string]cid.Cid
//		codes, err = node.StateActorCodeCIDs(context.Background(), abinetwork.Version(network))
//		if err != nil {
//			return
//		}
//		if len(codes) == 0 {
//			err = fmt.Errorf("codes is empty")
//			return
//		}
//		n.networkActorTypes[network] = map[string]string{}
//		for k, v := range codes {
//			//log.Debugf("register code, network: %d code: %s type: %s", network, v, k)
//			n.networkActorTypes[network][v.String()] = k
//		}
//	}
//
//	n.maxNetworkVersion = int64(nv)
//
//	return
//}
//
//func (n *networkActors) GetActorCode(network int64, t string) (code cid.Cid) {
//	n.lock.Lock()
//	defer func() {
//		n.lock.Unlock()
//	}()
//
//	if n.networkActorTypes == nil {
//		panic(fmt.Errorf("networkActorTypes is nil"))
//	}
//
//	v, ok := n.networkActorTypes[network]
//	if !ok {
//		panic(fmt.Errorf("network %d actors not register", network))
//	}
//
//	for kk, vv := range v {
//		if vv == t {
//			var err error
//			code, err = cid.Parse(kk)
//			if err != nil {
//				panic(err)
//			}
//			return
//		}
//	}
//
//	return
//}
//
//func (n *networkActors) GetActorType(code cid.Cid) (t string, err error) {
//	n.lock.Lock()
//	defer func() {
//		n.lock.Unlock()
//	}()
//
//	if n.networkActorTypes == nil {
//		err = fmt.Errorf("networkActorTypes is nil")
//		return
//	}
//
//	for network := n.maxNetworkVersion; network >= 0; network-- {
//		v, ok := n.networkActorTypes[network]
//		if !ok {
//			continue
//		}
//		t, ok = v[code.String()]
//		if !ok {
//			continue
//		} else {
//			break
//		}
//	}
//	if t == "" {
//		err = fmt.Errorf("cant't find code: %s type", code)
//		return
//	}
//	return
//}
//
//func (n *networkActors) IsStoragePower(code cid.Cid) (ok bool, err error) {
//
//	t, err := n.GetActorType(code)
//	if err != nil {
//		return
//	}
//	if t == manifest.PowerKey {
//		ok = true
//	}
//
//	return
//}
//
//func (n *networkActors) IsStorageMiner(code cid.Cid) (ok bool, err error) {
//
//	t, err := n.GetActorType(code)
//	if err != nil {
//		return
//	}
//	if t == manifest.MinerKey {
//		ok = true
//	}
//
//	return
//}
//
//func (n *networkActors) IsMultisigActor(code cid.Cid) (ok bool, err error) {
//
//	t, err := n.GetActorType(code)
//	if err != nil {
//		return
//	}
//	if t == manifest.MultisigKey {
//		ok = true
//	}
//
//	return
//}
//
//func (n *networkActors) IsAcceptedMessage(code cid.Cid, method abi.MethodNum) (ok bool, err error) {
//
//	t, err := n.GetActorType(code)
//	if err != nil {
//		return
//	}
//
//	if !code.Defined() {
//		return
//	}
//
//	if method == builtin.MethodSend {
//		ok = true
//		return
//	}
//
//	switch t {
//	case manifest.InitKey, manifest.MultisigKey, manifest.PowerKey, manifest.AccountKey:
//		ok = true
//		return
//	case manifest.MinerKey:
//		switch method {
//		case builtin8.MethodsMiner.WithdrawBalance,
//			builtin8.MethodsMiner.ChangeOwnerAddress,
//			builtin8.MethodsMiner.ChangeWorkerAddress,
//			builtin8.MethodsMiner.ConfirmUpdateWorkerKey,
//			builtin8.MethodsMiner.TerminateSectors,
//			builtin8.MethodsMiner.ChangePeerID,
//			builtin8.MethodsMiner.ExtendSectorExpiration,
//			builtin8.MethodsMiner.ChangeMultiaddrs:
//			ok = true
//			return
//		}
//	}
//
//	return
//}

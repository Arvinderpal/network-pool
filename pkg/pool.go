package pool

import (
	"fmt"
	"net"

	"github.com/apcera/continuum/common/fqn"
	"github.com/apcera/continuum/common/ipam"
	"github.com/apcera/continuum/util/netutil"
	"github.com/apcera/util/uuid"
)

// TODO(awander): move this to fqn.go eventually
const POOL_RESOURCE = "network"

// we don't allow arbitrarly large networks
// currently cap max networks per pool at 2^12=4K
const maxNetSize = 1 << 12

type PoolType int

func (t PoolType) String() string {
	switch t {
	case SHARED_POOL:
		return "shared"
	case PRIVATE_POOL:
		return "private"
	case MIX_POOL:
		return "mix"
	}
	return ""
}

const (
	SHARED_POOL PoolType = iota
	PRIVATE_POOL
	MIX_POOL
)

type NetworkType int

func (t NetworkType) String() string {
	switch t {
	case SHARED_NETWORK:
		return "shared"
	case PRIVATE_NETWORK:
		return "private"
	}
	return ""
}

const (
	SHARED_NETWORK NetworkType = iota
	PRIVATE_NETWORK
)

type SubnetRequest struct {
	Subnet string // desired subnet if any
	Type   NetworkType
}

type PoolRequest struct {
	FQN        *fqn.FQN
	Name       string
	Subnet     string
	maxSubnets int
	Type       PoolType
	TestOnly   bool
}

type SubnetKey string

// SubnetData for each subnet associated with this pool.
// Currently, we only support a single prefix/subnet per pool:
// e.g. 192.168.0.0/16. In the case of shared networks, this is further
// carved into smaller ranges for use by individual networks. In the case
// of private networks, all networks will use the 192.168.0.0/16 subnet.
// In the future, we may support multiple prefixes/subnets per pool.
// e.g. 192.168.0.0/16, 10.0.0.0/8 ... thus the SubnetKey is defined and
// a map is used inside the Pool object.
type SubnetData struct {
	Prefix           net.IP
	PrefixBitsLength int
	// Bits available for subnet allocation
	SubnetBitsLength int
	// Allocated addresses in each subnet
	Addresses *netutil.BitVector
}

type Pool struct {
	// Unique fully qualified name of the Network Pool
	FQN  *fqn.FQN `json:""`
	Name string   `json:""`
	UUID string   `json:""`
	Type PoolType

	Subnets map[SubnetKey]*SubnetData `json:""`
	// default SubnetKey; if > 1 subnets are specified, we need a default one
	defaultSubnet SubnetKey `json:""`
	// Networks associated with this Pool
	// Networks map[string]NetworkRef
	// For unit testing
	TestOnly bool `json:""`
}

func NewPool(poolReq *PoolRequest) (*Pool, error) {

	err := poolReq.validatePoolRequest()
	if err != nil {
		return nil, err
	}

	fmt.Printf("Creating New Pool %s\n", poolReq.Name)

	pool := &Pool{
		FQN:           poolReq.FQN,
		Name:          poolReq.Name,
		UUID:          uuid.Variant4().String(),
		Type:          poolReq.Type,
		defaultSubnet: SubnetKey(poolReq.Subnet),
		TestOnly:      poolReq.TestOnly,
	}
	pool.Subnets = make(map[SubnetKey]*SubnetData)

	subnetData := &SubnetData{}

	_, net, _ := net.ParseCIDR(poolReq.Subnet) // ignore err; already checked during validation
	// Seems that PraseCIDR return a 4byte IP, however, we always operate on the 16byte form, so To16() is necessary
	subnetData.Prefix = net.IP.To16()
	subnetData.PrefixBitsLength, _ = net.Mask.Size()
	subnetData.SubnetBitsLength = subnetBitsNeeded(poolReq.maxSubnets)
	subnetData.Addresses = allocateBitVector(poolReq.maxSubnets)

	// currently, we support only one subnet per pool which is marked as the default
	pool.Subnets[pool.defaultSubnet] = subnetData
	return pool, nil
}

func (p *Pool) ReserveSubnet(subnetReq *SubnetRequest) (*net.IPNet, error) {

	err := subnetReq.validateSubnetRequest()
	if err != nil {
		return nil, err
	}
	var k SubnetKey
	if subnetReq.Subnet == "" {
		k = p.defaultSubnet
	} else {
		k = SubnetKey(subnetReq.Subnet)
	}
	data, ok := p.Subnets[k]
	if !ok {
		return nil, fmt.Errorf("No %s subnet found in pool %s/%s ", subnetReq.Subnet, p.FQN, p.UUID)
	}
	net, err := p.internalReserveSubnet(data)
	return net, nil
}

func (p *Pool) internalReserveSubnet(data *SubnetData) (*net.IPNet, error) {

	bv := data.Addresses

	offset := netutil.FindIndexOfFirstFreeBit(bv)
	if offset < 0 {
		return nil, &ipam.ErrNoAvailableSubnet{}
	}
	fmt.Printf("found free subnet. offset: %v\n", offset)

	reserveSubnet := computeSubnetFromOffset(data.Prefix,
		data.PrefixBitsLength,
		data.SubnetBitsLength,
		uint32(offset))

	subnetMask := computeSubnetMask(data.PrefixBitsLength + data.SubnetBitsLength)
	subnetAddress := &net.IPNet{
		IP:   reserveSubnet,
		Mask: subnetMask,
	}
	fmt.Printf("Reserved Subnet Address %v , mask %v\n", subnetAddress, subnetMask)

	// set the bit in the bitvector to mark reservation
	bv.Set(1, uint(offset))
	return subnetAddress, nil
}

func (p *Pool) FreeSubnet(subnetReq *SubnetRequest) error {

	err := subnetReq.validateSubnetRequest()
	if err != nil {
		return err
	}

	return nil
}

func (pi *PoolRequest) validatePoolRequest() error {

	if pi.Type != SHARED_POOL && pi.Type != PRIVATE_POOL && pi.Type != MIX_POOL {
		return fmt.Errorf("Unknown pool type %s", pi.Type)
	}

	_, net, err := net.ParseCIDR(pi.Subnet)
	if err != nil || net == nil {
		return &ipam.ErrInvalidSubnet{}
	}

	if pi.maxSubnets > maxNetSize || pi.maxSubnets <= 0 {
		return fmt.Errorf("Maximum networks per pool must be lesss than %d and greather than 0; %d requested", maxNetSize, pi.maxSubnets)
	}

	// FQN
	if pi.FQN == nil {
		return fmt.Errorf("Pool requires an FQN.")
	} else if err := pi.FQN.Validate(false); err != nil {
		return fmt.Errorf("Invalid FQN: %s", err.Error())
	}
	if pi.FQN.LocalNameString() == "" {
		return fmt.Errorf("Pool cannot be created without a local name")
	}
	return nil
}

func (s *SubnetRequest) validateSubnetRequest() error {
	if s.Type != SHARED_NETWORK && s.Type != PRIVATE_NETWORK {
		return fmt.Errorf("Unknown network type %s", s.Type)
	}
	if s.Subnet != "" {
		_, net, err := net.ParseCIDR(s.Subnet)
		if err != nil || net == nil {
			return &ipam.ErrInvalidSubnet{}
		}
	}
	return nil
}

func allocateBitVector(maxNetworks int) *netutil.BitVector {
	var bytesNeeded int
	if maxNetworks%8 == 0 {
		bytesNeeded = maxNetworks / 8
	} else {
		bytesNeeded = (maxNetworks / 8) + 1
	}

	bv := &netutil.BitVector{}
	bv.Data = make([]byte, bytesNeeded)
	bv.Length = maxNetworks
	return bv
}

// subnetBitsNeeded is a util method to find the number of bits need for the
// subnet portion of the ipv4 address. for example, if the user desires, 5
// networks, we will need 3 bits; even with 8 networks, we will need 3 bits;
// however, if 9 networks are specified, we need 4 bits.
func subnetBitsNeeded(maxNetworks int) int {
	bitsNeeded := 0
	tmp := maxNetworks
	for {
		if tmp <= 1 {
			break
		}
		tmp = tmp >> 1
		bitsNeeded++
	}
	tmpMaxNetworks := 1 << uint(bitsNeeded)
	if maxNetworks <= tmpMaxNetworks {
		// maxnetworks is an exact power of 2
		// e.g. maxNetworks=4; we need only need 2 bits to handle this
		return bitsNeeded
	} else {
		// e.g. maxNetworks=5, we need 3 bits to handle this
		bitsNeeded++
		return bitsNeeded
	}
}
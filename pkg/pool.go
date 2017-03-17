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
	}
	return ""
}

const (
	SHARED_POOL PoolType = iota
	PRIVATE_POOL
)

type SubnetRequest struct {
	Subnet string // desired subnet if any (or subnet to be freed)
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
// carved into smaller ranges for use by individual networks. In the above
// example, all private networks will use the 192.168.0.0/16 subnet.
// In the future, we may support multiple prefixes/subnets per pool.
// e.g. 192.168.0.0/16, 10.0.0.0/8 ...
type SubnetData struct {
	Prefix           net.IP `json:""`
	PrefixBitsLength int    `json:""`
	// Bits available for subnet allocation
	SubnetBitsLength int `json:""`
	// Allocated addresses in each subnet
	Addresses *netutil.BitVector `json:""`
}

type Pool struct {
	// Unique fully qualified name of the Network Pool
	FQN     *fqn.FQN `json:""`
	Name    string   `json:""`
	UUID    string   `json:""`
	Type    PoolType
	Subnets *SubnetData `json:""`
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

	pool := &Pool{
		FQN:      poolReq.FQN,
		Name:     poolReq.Name,
		UUID:     uuid.Variant4().String(),
		Type:     poolReq.Type,
		TestOnly: poolReq.TestOnly,
	}

	subnetData := &SubnetData{}

	_, net, _ := net.ParseCIDR(poolReq.Subnet) // ignore err; already checked during validation
	// Seems that PraseCIDR return a 4byte IP, however, we always operate on the 16byte form, so To16() is necessary
	subnetData.Prefix = net.IP.To16()
	subnetData.PrefixBitsLength, _ = net.Mask.Size()
	subnetData.SubnetBitsLength = subnetBitsNeeded(poolReq.maxSubnets)
	subnetData.Addresses = allocateBitVector(poolReq.maxSubnets)
	// currently, we support only one subnet per pool which is marked as the default
	pool.Subnets = subnetData
	return pool, nil
}

func (p *Pool) ReserveSubnet(subnetReq *SubnetRequest) (*net.IPNet, error) {

	err := subnetReq.validateSubnetRequest()
	if err != nil {
		return nil, err
	}
	if p.Type == PRIVATE_POOL {
		// For private networks, the subnet is the pool prefix (e.g. 10/8)
		return &net.IPNet{
			IP:   p.Subnets.Prefix,
			Mask: computeSubnetMask(p.Subnets.PrefixBitsLength),
		}, nil
	}
	// For shared networks, we need to reserve a subnet
	net, err := p.internalReserveSubnet(p.Subnets)
	return net, nil
}

func (p *Pool) internalReserveSubnet(data *SubnetData) (*net.IPNet, error) {

	bv := data.Addresses

	offset := netutil.FindIndexOfFirstFreeBit(bv)
	if offset < 0 {
		return nil, &ipam.ErrNoAvailableSubnet{}
	}

	reserveSubnet := computeSubnetFromOffset(data.Prefix,
		data.PrefixBitsLength,
		data.SubnetBitsLength,
		uint32(offset))

	subnetMask := computeSubnetMask(data.PrefixBitsLength + data.SubnetBitsLength)
	subnetAddress := &net.IPNet{
		IP:   reserveSubnet,
		Mask: subnetMask,
	}

	// set the bit in the bitvector to mark reservation
	bv.Set(1, uint(offset))
	return subnetAddress, nil
}

func (p *Pool) FreeSubnet(subnetReq *SubnetRequest) error {

	err := subnetReq.validateSubnetRequest()
	if err != nil {
		return err
	}
	if p.Type == PRIVATE_POOL {
		// Nothing to do here yet... perhaps remove NetworkRef
		return nil
	}

	// TODO(awander): add a check that subnetReq.Subnet is within range of
	// p.Subnets.Prefix (use net.Contains())
	err = p.internalFreeSubnet(p.Subnets, subnetReq.Subnet)
	return nil
}

func (p *Pool) internalFreeSubnet(data *SubnetData, subnetStr string) error {

	bv := data.Addresses

	_, ipnet, _ := net.ParseCIDR(subnetStr) // no error checks; validation already happened in FreeSubnet

	// find the offset in the bitvector for this subnet
	offset := computeOffsetFromSubnet(
		ipnet.IP.To16(),
		data.PrefixBitsLength,
		data.SubnetBitsLength)

	if offset > uint(data.Addresses.Length) {
		// TODO(awander): log this error and return ipam.ErrIpamInternalConfigError{}
		return fmt.Errorf("Internal error: offset %v exceeds subnet size of %v", offset, data.Addresses.Length)
	}

	// fmt.Printf("found subnet offset: %v value: %v", offset, bv.Get(offset))
	// set the bit in the bitvector to 0 to mark it as free
	bv.Set(0, offset)

	return nil
}

func (pi *PoolRequest) validatePoolRequest() error {

	if pi.Type != SHARED_POOL && pi.Type != PRIVATE_POOL {
		return fmt.Errorf("Unknown pool type %s", pi.Type)
	}

	_, net, err := net.ParseCIDR(pi.Subnet)
	if err != nil || net == nil {
		return &ipam.ErrInvalidSubnet{}
	}

	if pi.Type == SHARED_POOL {
		if pi.maxSubnets > maxNetSize || pi.maxSubnets <= 0 {
			return fmt.Errorf("Maximum networks per pool must be lesss than %d and greather than 0; %d requested", maxNetSize, pi.maxSubnets)
		}
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
	if s.Subnet != "" {
		_, net, err := net.ParseCIDR(s.Subnet)
		if err != nil || net == nil {
			return &ipam.ErrInvalidSubnet{}
		}
	}
	return nil
}

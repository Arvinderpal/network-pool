package pool

import (
	"net"
	"testing"

	"github.com/apcera/continuum/common/fqn"
	"github.com/apcera/continuum/util/netutil"
)

var testPoolReqs = []struct {
	preq *PoolRequest
}{
	{
		&PoolRequest{
			FQN:        fqn.Build(POOL_RESOURCE, "::", "pool-0"),
			Name:       "pool-0",
			Subnet:     "10.0.0.0/8",
			maxSubnets: 129,
			Type:       SHARED_POOL,
			TestOnly:   true,
		},
	},
	{
		&PoolRequest{
			FQN:        fqn.Build(POOL_RESOURCE, "::", "pool-1"),
			Name:       "pool-1",
			Subnet:     "128.0.0.0/7",
			maxSubnets: 3,
			Type:       SHARED_POOL,
			TestOnly:   true,
		},
	},
}

func createTestPools() ([]*Pool, error) {
	var pools []*Pool
	var err error
	pools = make([]*Pool, len(testPoolReqs))
	for i, req := range testPoolReqs {
		pools[i], err = NewPool(req.preq)
		if err != nil {
			return nil, err
		}
	}
	return pools, nil
}

func TestNewPools(t *testing.T) {

	_, err := createTestPools()
	if err != nil {
		t.Fatalf("InitPool failed: %s", err)
	}
}

func TestSubnetData(t *testing.T) {

	var tests = []struct {
		in  *PoolRequest
		out *SubnetData
	}{
		{
			&PoolRequest{
				FQN:        fqn.Build(POOL_RESOURCE, "::", "pool-0"),
				Name:       "pool-0",
				Subnet:     "10.0.0.0/8",
				maxSubnets: 128,
				Type:       SHARED_POOL,
				TestOnly:   true,
			},
			&SubnetData{
				Prefix:           net.ParseIP("10.0.0.0"),
				PrefixBitsLength: 8,
				SubnetBitsLength: 7,
				Addresses: &netutil.BitVector{
					Data:   make([]byte, 128/8),
					Length: 128,
				},
			},
		},
	}

	for _, tt := range tests {
		pool, err := NewPool(tt.in)
		if err != nil {
			t.Fatalf("InitPool failed: %s", err)
		}
		res := pool.Subnets
		if !res.Prefix.Equal(tt.out.Prefix) {
			t.Errorf("expect %v got %v ", tt.out.Prefix, res.Prefix)
		}
		if res.PrefixBitsLength != tt.out.PrefixBitsLength {
			t.Errorf("expect %v got %v ", tt.out.PrefixBitsLength, res.PrefixBitsLength)
		}
		if res.SubnetBitsLength != tt.out.SubnetBitsLength {
			t.Errorf("expect %v got %v ", tt.out.SubnetBitsLength, res.SubnetBitsLength)
		}
		if res.Addresses.Length != tt.out.Addresses.Length {
			t.Errorf("expect %v got %v ", tt.out.Addresses.Length, res.Addresses.Length)
		}
		if len(res.Addresses.Data) != len(tt.out.Addresses.Data) {
			t.Errorf("expect %v got %v ", len(tt.out.Addresses.Data), len(res.Addresses.Data))
		}
	}
}

func TestReserveSubnetAndFreeSubnet(t *testing.T) {

	pools, err := createTestPools()
	if err != nil {
		t.Fatalf("createTestPools failed: %s", err)
	}

	var tests = []struct {
		in_subnetReq   *SubnetRequest
		in_testPoolIdx int
		out            *net.IPNet
	}{
		{
			&SubnetRequest{
				Subnet: "", Type: SHARED_NETWORK},
			0, // in_testPoolIdx
			&net.IPNet{
				IP:   net.ParseIP("10.0.0.0"),
				Mask: net.IPv4Mask(0xff, 0xff, 0x0, 0x0)},
		},
		{
			&SubnetRequest{
				Subnet: "", Type: SHARED_NETWORK},
			0, // in_testPoolIdx
			&net.IPNet{
				IP:   net.ParseIP("10.1.0.0"),
				Mask: net.IPv4Mask(0xff, 0xff, 0x0, 0x0)},
		},
		{
			&SubnetRequest{
				Subnet: "", Type: SHARED_NETWORK},
			1, // in_testPoolIdx
			&net.IPNet{
				IP:   net.ParseIP("128.0.0.0"),
				Mask: net.IPv4Mask(0xff, 0x80, 0x0, 0x0)},
		},
		{
			&SubnetRequest{
				Subnet: "", Type: SHARED_NETWORK},
			1, // in_testPoolIdx
			&net.IPNet{
				IP:   net.ParseIP("128.128.0.0"),
				Mask: net.IPv4Mask(0xff, 0x80, 0x0, 0x0)},
		},
		{
			&SubnetRequest{
				Subnet: "", Type: SHARED_NETWORK},
			1, // in_testPoolIdx
			&net.IPNet{
				IP:   net.ParseIP("129.0.0.0"),
				Mask: net.IPv4Mask(0xff, 0x80, 0x0, 0x0)},
		},
	}

	for _, tt := range tests {
		subnet, err := pools[tt.in_testPoolIdx].ReserveSubnet(tt.in_subnetReq)
		if err != nil {
			t.Errorf("error: %s", err)
		} else if !subnet.IP.Equal(tt.out.IP) || subnet.Mask.String() != tt.out.Mask.String() {
			t.Errorf("for input %+v expectd %s got %s", tt, tt.out, subnet)
		}
	}

	// Free the Subnet we allocated above
	var tests_freeSubnet = []struct {
		in_subnet      *SubnetRequest
		in_testPoolIdx int
		out_offset     uint
	}{
		{&SubnetRequest{tests[0].out.String(), SHARED_NETWORK}, tests[0].in_testPoolIdx, 0},
		{&SubnetRequest{tests[1].out.String(), SHARED_NETWORK}, tests[1].in_testPoolIdx, 1},
		{&SubnetRequest{tests[2].out.String(), SHARED_NETWORK}, tests[2].in_testPoolIdx, 0},
		{&SubnetRequest{tests[3].out.String(), SHARED_NETWORK}, tests[3].in_testPoolIdx, 1},
		{&SubnetRequest{tests[4].out.String(), SHARED_NETWORK}, tests[4].in_testPoolIdx, 2},
	}
	for _, tt := range tests_freeSubnet {
		bv := pools[tt.in_testPoolIdx].Subnets.Addresses
		if bv.Get(tt.out_offset) != 1 {
			t.Errorf("for input %+v expectd bit at position %v to be 1 (set) but it's %v", tt, tt.out_offset, bv.Get(tt.out_offset))
		}
		err := pools[tt.in_testPoolIdx].FreeSubnet(tt.in_subnet)
		if err != nil {
			t.Errorf("error: %s", err)
		} else if bv.Get(tt.out_offset) != 0 {
			t.Errorf("for input %+v expectd bit at position %v to be 0 (unset) but it's %v", tt, tt.out_offset, bv.Get(tt.out_offset))
		}
	}
}

func TestComputeSubnetFromOffset(t *testing.T) {

	var tests = []struct {
		in_prefix     net.IP
		in_prefixLen  int
		in_subnetLen  int
		in_subnetBits uint32
		out           net.IP
	}{
		{net.ParseIP("192.168.0.0"), 16, 8, 42, net.ParseIP("192.168.42.0")},
		{net.ParseIP("192.168.0.0"), 16, 9, 256, net.ParseIP("192.168.128.0")},
		{net.ParseIP("192.168.0.0"), 16, 9, 257, net.ParseIP("192.168.128.128")},
		{net.ParseIP("192.168.0.0"), 16, 9, 255, net.ParseIP("192.168.127.128")},
		{net.ParseIP("172.169.128.0"), 20, 4, 1, net.ParseIP("172.169.129.0")},
		{net.ParseIP("172.169.128.0"), 20, 4, 2, net.ParseIP("172.169.130.0")},
		{net.ParseIP("172.169.128.0"), 20, 4, 8, net.ParseIP("172.169.136.0")},
		{net.ParseIP("172.169.128.0"), 20, 5, 1, net.ParseIP("172.169.128.128")},
		{net.ParseIP("172.169.128.0"), 20, 5, 2, net.ParseIP("172.169.129.0")},
		{net.ParseIP("254.0.0.0"), 7, 11, 1, net.ParseIP("254.0.64.0")},
		{net.ParseIP("254.0.0.0"), 7, 11, 2047, net.ParseIP("255.255.192.0")},
		{net.ParseIP("254.0.0.0"), 7, 11, 2046, net.ParseIP("255.255.128.0")},
		{net.ParseIP("10.0.0.0"), 8, 8, 0, net.ParseIP("10.0.0.0")},
	}
	for _, tt := range tests {
		subnet := computeSubnetFromOffset(tt.in_prefix, tt.in_prefixLen, tt.in_subnetLen, tt.in_subnetBits)
		if !subnet.Equal(tt.out) {
			t.Errorf("for input %v expectd %s got %s", tt, tt.out, subnet)
		}
	}
}

func TestComputeOffsetFromSubnet(t *testing.T) {

	var tests = []struct {
		in_subnet    net.IP
		in_prefixLen int
		in_subnetLen int
		out          int
	}{
		{net.ParseIP("192.168.42.0"), 16, 8, 42},
		{net.ParseIP("192.168.128.0"), 16, 9, 256},
		{net.ParseIP("192.168.128.128"), 16, 9, 257},
		{net.ParseIP("192.168.127.128"), 16, 9, 255},
		{net.ParseIP("172.169.129.0"), 20, 4, 1},
		{net.ParseIP("172.169.130.0"), 20, 4, 2},
		{net.ParseIP("172.169.136.0"), 20, 4, 8},
		{net.ParseIP("172.169.128.128"), 20, 5, 1},
		{net.ParseIP("172.169.129.0"), 20, 5, 2},
		{net.ParseIP("254.0.64.0"), 7, 11, 1},
		{net.ParseIP("255.255.192.0"), 7, 11, 2047},
		{net.ParseIP("255.255.128.0"), 7, 11, 2046},
		{net.ParseIP("10.0.0.0"), 8, 8, 0},
	}
	for _, tt := range tests {
		offset := computeOffsetFromSubnet(tt.in_subnet, tt.in_prefixLen, tt.in_subnetLen)
		if tt.out != int(offset) {
			t.Errorf("for input %+v expectd %v got %v", tt, tt.out, offset)
		}
	}
}

func TestSubnetBitsNeeded(t *testing.T) {

	var bitsNeededTests = []struct {
		in  int
		out int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 2},
		{4, 2},
		{5, 3},
		{8, 3},
		{9, 4},
		{256, 8},
	}

	for _, tt := range bitsNeededTests {
		bitsNeeded := subnetBitsNeeded(tt.in)
		if bitsNeeded != tt.out {
			t.Errorf("for input %v expectd %v got %v", tt.in, tt.out, bitsNeeded)

		}
	}

}

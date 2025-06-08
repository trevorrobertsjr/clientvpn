package utils

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type VPCResult struct {
	Vpc                   *ec2.Vpc
	InternetGateway       *ec2.InternetGateway
	PublicSubnets         map[string]*ec2.Subnet
	PrivateComputeSubnets map[string]*ec2.Subnet
	PrivateDbSubnets      map[string]*ec2.Subnet
	PrivateTgwSubnets     map[string]*ec2.Subnet
	DNS                   pulumi.StringOutput
}

type VPCArgs struct {
	NamePrefix string
	CIDRBlock  string
	AZs        []string
	Region     string
}

func getFirstTwoOctets(cidr string) (string, error) {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid CIDR format")
	}
	octets := strings.Split(parts[0], ".")
	if len(octets) < 2 {
		return "", fmt.Errorf("invalid IP address in CIDR")
	}
	return fmt.Sprintf("%s.%s", octets[0], octets[1]), nil
}

func CreateCustomVPC(ctx *pulumi.Context, args VPCArgs, opts ...pulumi.ResourceOption) (*VPCResult, error) {
	firstTwoOctets, err := getFirstTwoOctets(args.CIDRBlock)
	if err != nil {
		return nil, err
	}

	vpc, err := ec2.NewVpc(ctx, args.NamePrefix+"-vpc", &ec2.VpcArgs{
		CidrBlock:          pulumi.String(args.CIDRBlock),
		EnableDnsSupport:   pulumi.Bool(true),
		EnableDnsHostnames: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(args.NamePrefix + "-vpc"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	igw, err := ec2.NewInternetGateway(ctx, args.NamePrefix+"-igw", &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
		Tags: pulumi.StringMap{
			"Name": pulumi.String(args.NamePrefix + "-igw"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	publicRT, err := ec2.NewRouteTable(ctx, args.NamePrefix+"-public-rt", &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			&ec2.RouteTableRouteArgs{
				CidrBlock: pulumi.String("0.0.0.0/0"),
				GatewayId: igw.ID(),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String(args.NamePrefix + "-public-rt"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	thirdOctet := 0
	publicSubnets := make(map[string]*ec2.Subnet)
	privComputeSubnets := make(map[string]*ec2.Subnet)
	privComputeSubnetCidrs := make(map[string]string)
	privDbSubnets := make(map[string]*ec2.Subnet)
	privTgwSubnets := make(map[string]*ec2.Subnet)

	for _, az := range args.AZs {
		pubCidr := fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)
		pubSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-pub-subnet-compute-%s", args.NamePrefix, az), &ec2.SubnetArgs{
			VpcId:               vpc.ID(),
			CidrBlock:           pulumi.String(pubCidr),
			AvailabilityZone:    pulumi.String(fmt.Sprintf("%s%s", args.Region, az)),
			MapPublicIpOnLaunch: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-pub-subnet-compute-%s", args.NamePrefix, az)),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-public-rt-assoc-%s", args.NamePrefix, az), &ec2.RouteTableAssociationArgs{
			SubnetId:     pubSubnet.ID(),
			RouteTableId: publicRT.ID(),
		}, opts...)
		if err != nil {
			return nil, err
		}
		publicSubnets[az] = pubSubnet
		thirdOctet++

		privComputeCidr := fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)
		privComputeSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-priv-subnet-compute-%s", args.NamePrefix, az), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(privComputeCidr),
			AvailabilityZone: pulumi.String(fmt.Sprintf("%s%s", args.Region, az)),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-priv-subnet-compute-%s", args.NamePrefix, az)),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
		privComputeSubnets[az] = privComputeSubnet
		privComputeSubnetCidrs[az] = privComputeCidr
		thirdOctet++

		privDbCidr := fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)
		privDbSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-priv-subnet-db-%s", args.NamePrefix, az), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(privDbCidr),
			AvailabilityZone: pulumi.String(fmt.Sprintf("%s%s", args.Region, az)),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-priv-subnet-db-%s", args.NamePrefix, az)),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
		privDbSubnets[az] = privDbSubnet
		thirdOctet++

		tgwCidr := fmt.Sprintf("%s.%d.240/28", firstTwoOctets, thirdOctet)
		tgwSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("%s-priv-subnet-tgw-%s", args.NamePrefix, az), &ec2.SubnetArgs{
			VpcId:            vpc.ID(),
			CidrBlock:        pulumi.String(tgwCidr),
			AvailabilityZone: pulumi.String(fmt.Sprintf("%s%s", args.Region, az)),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-priv-subnet-tgw-%s", args.NamePrefix, az)),
			},
		}, opts...)
		if err != nil {
			return nil, err
		}
		privTgwSubnets[az] = tgwSubnet
		thirdOctet++
	}

	dns := pulumi.Sprintf("%s.0.2", firstTwoOctets)

	return &VPCResult{
		Vpc:                   vpc,
		InternetGateway:       igw,
		PublicSubnets:         publicSubnets,
		PrivateComputeSubnets: privComputeSubnets,
		PrivateDbSubnets:      privDbSubnets,
		PrivateTgwSubnets:     privTgwSubnets,
		DNS:                   dns,
	}, nil
}

func (v *VPCResult) AddTGWRoute(ctx *pulumi.Context, name string, destinationCidr string, tgwId pulumi.IDOutput, opts ...pulumi.ResourceOption) error {
	for az, subnet := range v.PrivateComputeSubnets {
		rt, err := ec2.NewRouteTable(ctx, fmt.Sprintf("%s-tgw-rt-%s", name, az), &ec2.RouteTableArgs{
			VpcId: v.Vpc.ID(),
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-tgw-rt-%s", name, az)),
			},
		}, opts...)
		if err != nil {
			return err
		}

		_, err = ec2.NewRoute(ctx, fmt.Sprintf("%s-tgw-route-%s", name, az), &ec2.RouteArgs{
			RouteTableId:         rt.ID(),
			DestinationCidrBlock: pulumi.String(destinationCidr),
			TransitGatewayId:     tgwId,
		}, opts...)
		if err != nil {
			return err
		}

		_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("%s-tgw-rt-assoc-%s", name, az), &ec2.RouteTableAssociationArgs{
			SubnetId:     subnet.ID(),
			RouteTableId: rt.ID(),
		}, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

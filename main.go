package main

import (
	"clientvpn/utils"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// BEGIN: Foundational Parameters
		// Providers for each region
		eastProvider, err := aws.NewProvider(ctx, "east", &aws.ProviderArgs{
			Region: pulumi.String("us-east-2"),
		})
		if err != nil {
			return err
		}
		westProvider, err := aws.NewProvider(ctx, "west", &aws.ProviderArgs{
			Region: pulumi.String("us-west-2"),
		})
		if err != nil {
			return err
		}
		// region := "us-east-2"
		azs := []string{"a", "b", "c"}
		eastRegion := "us-east-2"
		westRegion := "us-west-2"
		eastVpcCidr := "172.16.0.0/16"
		westVpcCidr := "172.17.0.0/16"
		clientCidrBlock := "10.255.252.0/22"
		serverCertificateArn := "arn:aws:acm:us-east-2:318168271290:certificate/9e709430-a008-4d6a-9599-265c3e5f24dc"
		samlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn"
		selfServiceSamlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn-self-service"

		amiUsEast2, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
			MostRecent: pulumi.BoolRef(true),
			Owners:     []string{"amazon"},
			Filters: []ec2.GetAmiFilter{
				{
					Name:   "name",
					Values: []string{"al2023-ami-*-arm64"},
				},
				{
					Name:   "architecture",
					Values: []string{"arm64"},
				},
				{
					Name:   "virtualization-type",
					Values: []string{"hvm"},
				},
			},
		},
			pulumi.Provider(eastProvider))

		if err != nil {
			return err
		}

		amiUsWest2, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
			MostRecent: pulumi.BoolRef(true),
			Owners:     []string{"amazon"},
			Filters: []ec2.GetAmiFilter{
				{
					Name:   "name",
					Values: []string{"al2023-ami-*-arm64"},
				},
				{
					Name:   "architecture",
					Values: []string{"arm64"},
				},
				{
					Name:   "virtualization-type",
					Values: []string{"hvm"},
				},
			},
		},
			pulumi.Provider(westProvider))

		if err != nil {
			return err
		}
		// END: Foundational Parameters

		// Create VPC & EC2 in us-east-2
		eastVpc, err := utils.CreateCustomVPC(ctx, utils.VPCArgs{
			NamePrefix: "east",
			CIDRBlock:  eastVpcCidr,
			AZs:        azs,
			Region:     eastRegion,
		}, pulumi.Provider(eastProvider))

		if err != nil {
			return err
		}

		eastInstance, err := utils.CreateEC2WithICMPAccess(ctx, utils.InstanceArgs{
			Name:           "east-instance",
			VpcId:          eastVpc.Vpc.ID(),
			SubnetId:       eastVpc.PrivateComputeSubnets["a"].ID(),
			CidrForIngress: eastVpcCidr,
			AmiId:          amiUsEast2.Id,
		}, pulumi.Provider(eastProvider))
		if err != nil {
			return err
		}

		// Create VPC & EC2 in us-west-2
		westVpc, err := utils.CreateCustomVPC(ctx, utils.VPCArgs{
			NamePrefix: "west",
			CIDRBlock:  westVpcCidr,
			AZs:        azs,
			Region:     westRegion,
		}, pulumi.Provider(westProvider))
		if err != nil {
			return err
		}

		westInstance, err := utils.CreateEC2WithICMPAccess(ctx, utils.InstanceArgs{
			Name:           "west-instance",
			VpcId:          westVpc.Vpc.ID(),
			SubnetId:       westVpc.PrivateComputeSubnets["a"].ID(),
			CidrForIngress: eastVpcCidr,
			AmiId:          amiUsWest2.Id,
		}, pulumi.Provider(westProvider))
		if err != nil {
			return err
		}

		// Create Transit Gateways in both regions
		eastTgw, err := utils.CreateTransitGateway(ctx, "east-tgw", pulumi.Provider(eastProvider))

		if err != nil {
			return err
		}

		westTgw, err := utils.CreateTransitGateway(ctx, "west-tgw", pulumi.Provider(westProvider))

		if err != nil {
			return err
		}

		// Create TGW peering attachment
		_, _, err = utils.CreateTGWPeeringAttachment(ctx, "tgw-peering",
			eastTgw,
			westTgw,
			westRegion,
			pulumi.Provider(eastProvider),
			pulumi.Provider(westProvider))
		if err != nil {
			return err
		}

		// Attach VPCs to TGWs
		_, err = utils.AttachVPCsToTGW(ctx, eastTgw, eastRegion, []*utils.VPCResult{eastVpc}, pulumi.Provider(eastProvider))
		if err != nil {
			return err
		}

		_, err = utils.AttachVPCsToTGW(ctx, westTgw, westRegion, []*utils.VPCResult{westVpc}, pulumi.Provider(westProvider))
		if err != nil {
			return err
		}

		// Add TGW routes
		err = eastVpc.AddTGWRoute(ctx, "eastVpcAddWestVpcRoute", westVpcCidr, eastTgw.ID(), pulumi.Provider(eastProvider))
		if err != nil {
			return err
		}
		err = westVpc.AddTGWRoute(ctx, "westVpcAddEastVpcRoute", eastVpcCidr, westTgw.ID(), pulumi.Provider(westProvider))
		if err != nil {
			return err
		}

		// VPN in eastVPC
		cVpnResult, err := utils.CreateClientVPN(ctx, utils.VPNArgs{
			VpcId:                eastVpc.Vpc.ID(),
			PrivateComputeSubnet: eastVpc.PrivateComputeSubnets["a"].ID(),
			DNS:                  eastVpc.DNS,
			ClientCidrBlock:      clientCidrBlock,
			ServerCertificateArn: serverCertificateArn,
			SamlProviderArn:      samlProviderArn,
			SelfServiceSamlArn:   selfServiceSamlProviderArn,
		},
			pulumi.Provider(eastProvider))
		if err != nil {
			return err
		}

		// Add route to the other VPC's subnet through TGW
		_, err = utils.AddClientVPNRoute(ctx, "vpnRouteToOtherVPC",
			cVpnResult,
			westVpcCidr,                             // CIDR of your other VPC or specific subnet
			eastVpc.PrivateComputeSubnets["a"].ID(), // A subnet ID in your target VPC
			pulumi.Provider(eastProvider),
		)
		if err != nil {
			return err
		}

		// Export the private IPs
		ctx.Export("eastInstanceId", eastInstance.ID())
		ctx.Export("westInstanceId", westInstance.ID())
		ctx.Export("eastInstancePrivateIP", eastInstance.PrivateIp)
		ctx.Export("westInstancePrivateIP", westInstance.PrivateIp)

		return nil
	})
}

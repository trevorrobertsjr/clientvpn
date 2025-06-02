package main

import (
	"clientvpn/utils"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		region := "us-east-2"
		azs := []string{"a", "b", "c"}
		vpc1Cidr := "172.16.0.0/16"
		vpc2Cidr := "172.17.0.0/16"
		clientCidrBlock := "10.255.252.0/22"
		serverCertificateArn := "arn:aws:acm:us-east-2:318168271290:certificate/9e709430-a008-4d6a-9599-265c3e5f24dc"
		samlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn"
		selfServiceSamlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn-self-service"
		ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
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
		})
		if err != nil {
			return err
		}

		// Create VPC 1 and VPC 2
		vpc1, err := utils.CreateCustomVPC(ctx, utils.VPCArgs{
			NamePrefix: "vpc1",
			CIDRBlock:  vpc1Cidr,
			AZs:        azs,
			Region:     region,
		})
		if err != nil {
			return err
		}

		vpc2, err := utils.CreateCustomVPC(ctx, utils.VPCArgs{
			NamePrefix: "vpc2",
			CIDRBlock:  vpc2Cidr,
			AZs:        azs,
			Region:     region,
		})
		if err != nil {
			return err
		}

		// Transit Gateway
		tgw, err := utils.CreateTransitGateway(ctx, "tgw")
		if err != nil {
			return err
		}

		_, err = utils.AttachVPCsToTGW(ctx, tgw, vpc1, vpc2)
		if err != nil {
			return err
		}

		// Add TGW routes
		err = vpc1.AddTGWRoute(ctx, "vpc1", vpc2Cidr, tgw.ID())
		if err != nil {
			return err
		}
		err = vpc2.AddTGWRoute(ctx, "vpc2", vpc1Cidr, tgw.ID())
		if err != nil {
			return err
		}

		// VPN in VPC1
		_, err = utils.CreateClientVPN(ctx, utils.VPNArgs{
			VpcId:                vpc1.Vpc.ID(),
			PrivateComputeSubnet: vpc1.PrivateComputeSubnets["a"].ID(),
			DNS:                  vpc1.DNS,
			ClientCidrBlock:      clientCidrBlock,
			ServerCertificateArn: serverCertificateArn,
			SamlProviderArn:      samlProviderArn,
			SelfServiceSamlArn:   selfServiceSamlProviderArn,
		})
		if err != nil {
			return err
		}

		// EC2 in VPC1 with ICMP from VPN CIDR and VPC2 CIDR
		err = utils.CreateEC2WithICMPAccess(ctx, utils.InstanceArgs{
			Name:           "vpc1-instance",
			VpcId:          vpc1.Vpc.ID(),
			SubnetId:       vpc1.PrivateComputeSubnets["a"].ID(),
			CidrForIngress: clientCidrBlock,
			AmiId:          ami.Id,
		})

		if err != nil {
			return err
		}
		// EC2 in VPC2 with ICMP from VPC1 CIDR
		err = utils.CreateEC2WithICMPAccess(ctx, utils.InstanceArgs{
			Name:           "vpc2-instance",
			VpcId:          vpc2.Vpc.ID(),
			SubnetId:       vpc2.PrivateComputeSubnets["a"].ID(),
			CidrForIngress: vpc1Cidr,
			AmiId:          ami.Id,
		})

		if err != nil {
			return err
		}

		return nil
	})
}

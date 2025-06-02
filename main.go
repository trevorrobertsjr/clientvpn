package main

import (
	"clientvpn/networking"

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
		vpc1, err := networking.CreateCustomVPC(ctx, networking.VPCArgs{
			NamePrefix: "vpc1",
			CIDRBlock:  vpc1Cidr,
			AZs:        azs,
			Region:     region,
		})
		if err != nil {
			return err
		}

		vpc2, err := networking.CreateCustomVPC(ctx, networking.VPCArgs{
			NamePrefix: "vpc2",
			CIDRBlock:  vpc2Cidr,
			AZs:        azs,
			Region:     region,
		})
		if err != nil {
			return err
		}

		// Transit Gateway
		tgw, err := networking.CreateTransitGateway(ctx, "tgw")
		if err != nil {
			return err
		}

		_, err = networking.AttachVPCsToTGW(ctx, tgw, vpc1, vpc2)
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
		_, err = networking.CreateClientVPN(ctx, networking.VPNArgs{
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

		// EC2 in VPC1 with ICMP from VPN CIDR
		vpc1InstanceSG, err := ec2.NewSecurityGroup(ctx, "vpc1-instance-sg", &ec2.SecurityGroupArgs{
			VpcId: vpc1.Vpc.ID(),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("icmp"),
					FromPort:   pulumi.Int(-1),
					ToPort:     pulumi.Int(-1),
					CidrBlocks: pulumi.StringArray{pulumi.String(clientCidrBlock)},
				},
			},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = ec2.NewInstance(ctx, "vpc1-instance", &ec2.InstanceArgs{
			Ami:                 pulumi.String(ami.Id),
			InstanceType:        pulumi.String("t4g.micro"),
			SubnetId:            vpc1.PrivateComputeSubnets["a"].ID(),
			VpcSecurityGroupIds: pulumi.StringArray{vpc1InstanceSG.ID()},
			Tags:                pulumi.StringMap{"Name": pulumi.String("vpc1-ec2")},
		})
		if err != nil {
			return err
		}

		// EC2 in VPC2 with ICMP from VPC1
		vpc2InstanceSG, err := ec2.NewSecurityGroup(ctx, "vpc2-instance-sg", &ec2.SecurityGroupArgs{
			VpcId: vpc2.Vpc.ID(),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("icmp"),
					FromPort:   pulumi.Int(-1),
					ToPort:     pulumi.Int(-1),
					CidrBlocks: pulumi.StringArray{pulumi.String(vpc1Cidr)},
				},
			},
			Egress: ec2.SecurityGroupEgressArray{
				&ec2.SecurityGroupEgressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				},
			},
		})
		if err != nil {
			return err
		}

		_, err = ec2.NewInstance(ctx, "vpc2-instance", &ec2.InstanceArgs{
			Ami:                 pulumi.String(ami.Id),
			InstanceType:        pulumi.String("t4g.micro"),
			SubnetId:            vpc2.PrivateComputeSubnets["a"].ID(),
			VpcSecurityGroupIds: pulumi.StringArray{vpc2InstanceSG.ID()},
			Tags:                pulumi.StringMap{"Name": pulumi.String("vpc2-ec2")},
		})
		if err != nil {
			return err
		}

		return nil
	})
}

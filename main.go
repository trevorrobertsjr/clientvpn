package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2clientvpn"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func getFirstTwoOctets(cidr string) (string, error) {
	// Split the CIDR block into its components (IP and prefix)
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid CIDR format")
	}

	// Split the IP address into its octets
	octets := strings.Split(parts[0], ".")
	if len(octets) < 2 {
		return "", fmt.Errorf("invalid IP address in CIDR")
	}

	// Return the first two octets joined by a dot
	return fmt.Sprintf("%s.%s", octets[0], octets[1]), nil
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// BEGIN Parameter List
		resourceNamePrefix := "blog-us-east-2"
		vpcCidrBlock := "172.16.0.0/16"
		firstTwoOctets, err := getFirstTwoOctets(vpcCidrBlock)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		vpcDns := fmt.Sprintf("%s.0.2", firstTwoOctets)
		clientCidrBlock := "10.255.252.0/22"
		// asn := 64512
		region := "us-east-2"
		azs := []string{"a", "b", "c"}
		// tgwCidrBlocks := []string{"172.16.255.208/28", "172.16.255.224/28", "172.16.255.240/28"}
		serverCertificateArn := "arn:aws:acm:us-east-2:318168271290:certificate/9e709430-a008-4d6a-9599-265c3e5f24dc"
		samlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn"
		selfServiceSamlProviderArn := "arn:aws:iam::318168271290:saml-provider/aws-client-vpn-self-service"
		// END Parameter List

		// BEGIN us-east-2 VPC Creation
		// Create a VPC
		vpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", resourceNamePrefix), &ec2.VpcArgs{
			CidrBlock:          pulumi.String(vpcCidrBlock),
			Tags:               pulumi.StringMap{"Name": pulumi.String(fmt.Sprintf("%s-vpc", resourceNamePrefix))},
			InstanceTenancy:    pulumi.String("default"),
			EnableDnsSupport:   pulumi.Bool(true),
			EnableDnsHostnames: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		// Create an Internet Gateway
		igw, err := ec2.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", resourceNamePrefix), &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
			Tags:  pulumi.StringMap{"Name": pulumi.String(fmt.Sprintf("%s-igw", resourceNamePrefix))},
		})
		if err != nil {
			return err
		}

		// Create a Public Route Table
		publicRouteTable, err := ec2.NewRouteTable(ctx, "public-route-table", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: igw.ID(),
				},
			},
			Tags: pulumi.StringMap{"Name": pulumi.String(fmt.Sprintf("%s-public-rt", resourceNamePrefix))},
		})
		if err != nil {
			return err
		}

		// Subnets and Route Table Associations
		publicComputeSubnetsDict := make(map[string]*ec2.Subnet)
		privateComputeSubnetsDict := make(map[string]*ec2.Subnet)
		privateDbSubnetsDict := make(map[string]*ec2.Subnet)
		var publicComputeSubnets []*ec2.Subnet
		var publicComputeSubnetCidrs []string
		thirdOctet := 0
		for _, az := range azs {
			publicComputeSubnetCidrs = append(publicComputeSubnetCidrs, fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet))
			publicSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("public-compute-%s", az), &ec2.SubnetArgs{
				VpcId:               vpc.ID(),
				CidrBlock:           pulumi.String(fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)),
				AvailabilityZone:    pulumi.String(fmt.Sprintf("%s%s", region, az)),
				MapPublicIpOnLaunch: pulumi.Bool(true),
				Tags: pulumi.StringMap{
					"Name": pulumi.String(fmt.Sprintf("%s-pub-subnet-compute-%s", resourceNamePrefix, az)),
				},
			})
			if err != nil {
				return err
			}
			thirdOctet++

			_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("public-rt-assoc-%s", az), &ec2.RouteTableAssociationArgs{
				SubnetId:     publicSubnet.ID(),
				RouteTableId: publicRouteTable.ID(),
			})
			if err != nil {
				return err
			}

			// Create a Public Route Table
			privateRouteTable, err := ec2.NewRouteTable(ctx, fmt.Sprintf("private-route-table-%s", az), &ec2.RouteTableArgs{
				VpcId: vpc.ID(),
				// Routes: ec2.RouteTableRouteArray{
				// 	&ec2.RouteTableRouteArgs{
				// 		CidrBlock: pulumi.String("0.0.0.0/0"),
				// 		GatewayId: igw.ID(),
				// 	},
				// },
				Tags: pulumi.StringMap{"Name": pulumi.String(fmt.Sprintf("%s-private-rt", resourceNamePrefix))},
			})
			if err != nil {
				return err
			}

			privateComputeSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("private-compute-%s", az), &ec2.SubnetArgs{
				VpcId:            vpc.ID(),
				CidrBlock:        pulumi.String(fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)),
				AvailabilityZone: pulumi.String(fmt.Sprintf("%s%s", region, az)),
				Tags: pulumi.StringMap{
					"Name": pulumi.String(fmt.Sprintf("%s-priv-subnet-compute-%s", resourceNamePrefix, az)),
				},
			})
			if err != nil {
				return err
			}
			thirdOctet++

			_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("private-compute-rt-assoc-%s", az), &ec2.RouteTableAssociationArgs{
				SubnetId:     privateComputeSubnet.ID(),
				RouteTableId: privateRouteTable.ID(),
			})
			if err != nil {
				return err
			}

			privateDbSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("private-db-%s", az), &ec2.SubnetArgs{
				VpcId:            vpc.ID(),
				CidrBlock:        pulumi.String(fmt.Sprintf("%s.%d.0/24", firstTwoOctets, thirdOctet)),
				AvailabilityZone: pulumi.String(fmt.Sprintf("%s%s", region, az)),
				Tags: pulumi.StringMap{
					"Name": pulumi.String(fmt.Sprintf("%s-priv-subnet-db-%s", resourceNamePrefix, az)),
				},
			})
			if err != nil {
				return err
			}
			thirdOctet++

			_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("private-db-rt-assoc-%s", az), &ec2.RouteTableAssociationArgs{
				SubnetId:     privateDbSubnet.ID(),
				RouteTableId: privateRouteTable.ID(),
			})
			if err != nil {
				return err
			}

			publicComputeSubnetsDict[az] = publicSubnet
			privateComputeSubnetsDict[az] = privateComputeSubnet
			privateDbSubnetsDict[az] = privateDbSubnet

			publicComputeSubnets = append(publicComputeSubnets, publicSubnet)

		}
		// END us-east-2 VPC Creation

		// BEGIN VPN Creation
		// Create a CloudWatch Log Group for the VPN Endpoint
		logGroup, err := cloudwatch.NewLogGroup(ctx, "clientvpnLogGroup", &cloudwatch.LogGroupArgs{
			RetentionInDays: pulumi.Int(7), // Keep logs for 7 days
		})
		if err != nil {
			return err
		}

		// Create a Security Group for the VPN Endpoint
		vpnSecurityGroup, err := ec2.NewSecurityGroup(ctx, "vpnSecurityGroup", &ec2.SecurityGroupArgs{
			VpcId:       vpc.ID(),
			Description: pulumi.String("Allow all inbound traffic"),
			Ingress: ec2.SecurityGroupIngressArray{
				&ec2.SecurityGroupIngressArgs{
					Protocol:   pulumi.String("-1"),
					FromPort:   pulumi.Int(0),
					ToPort:     pulumi.Int(0),
					CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
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

		// Create the Client VPN Endpoint
		vpnEndpoint, err := ec2clientvpn.NewEndpoint(ctx, "vpnEndpoint", &ec2clientvpn.EndpointArgs{
			VpcId:                vpc.ID(),
			SecurityGroupIds:     pulumi.StringArray{vpnSecurityGroup.ID()},
			ClientCidrBlock:      pulumi.String(clientCidrBlock),
			DnsServers:           pulumi.StringArray{pulumi.String(vpcDns)},
			ServerCertificateArn: pulumi.String(serverCertificateArn),
			ConnectionLogOptions: &ec2clientvpn.EndpointConnectionLogOptionsArgs{
				Enabled:            pulumi.Bool(true),
				CloudwatchLogGroup: logGroup.Name,
			},
			AuthenticationOptions: ec2clientvpn.EndpointAuthenticationOptionArray{
				&ec2clientvpn.EndpointAuthenticationOptionArgs{
					Type:                       pulumi.String("federated-authentication"),
					SamlProviderArn:            pulumi.String(samlProviderArn),
					SelfServiceSamlProviderArn: pulumi.String(selfServiceSamlProviderArn),
				},
			},
			SplitTunnel: pulumi.Bool(true),
			Tags: pulumi.StringMap{
				"Name": pulumi.String("AWS SSO Client VPN"),
			},
		})
		if err != nil {
			return err
		}
		_, err = ec2clientvpn.NewNetworkAssociation(ctx, "vpnSubnetAssociation", &ec2clientvpn.NetworkAssociationArgs{
			ClientVpnEndpointId: vpnEndpoint.ID(),
			SubnetId:            privateComputeSubnetsDict["a"].ID(),
		})
		if err != nil {
			return err
		}
		// _, err = ec2clientvpn.NewAuthorizationRule(ctx, "vpnNetworkAuthorization", &ec2clientvpn.AuthorizationRuleArgs{
		// 	ClientVpnEndpointId: vpnEndpoint.ID(),
		// 	TargetNetworkCidr:   pulumi.String(publicComputeSubnets[0].CidrBlock),
		// 	AuthorizeAllGroups:  pulumi.Bool(true),
		// })
		_, err = ec2clientvpn.NewAuthorizationRule(ctx, "vpnNetworkAuthorization", &ec2clientvpn.AuthorizationRuleArgs{
			ClientVpnEndpointId: vpnEndpoint.ID(),
			TargetNetworkCidr: privateComputeSubnetsDict["a"].CidrBlock.ApplyT(func(cidrBlock *string) string {
				if cidrBlock == nil {
					panic("CIDR Block is nil")
				}
				return *cidrBlock
			}).(pulumi.StringOutput),
			AuthorizeAllGroups: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}
		// END VPN Creation

		// BEGIN EC2 for Reachability Test
		// Create a Security Group Rule for ICMP (ping) traffic from VPN security group
		_, err = ec2.NewSecurityGroupRule(ctx, "vpnSecurityGroupAllowICMP", &ec2.SecurityGroupRuleArgs{
			Type:                  pulumi.String("ingress"),
			SecurityGroupId:       vpnSecurityGroup.ID(),
			Protocol:              pulumi.String("icmp"),
			FromPort:              pulumi.Int(-1),
			ToPort:                pulumi.Int(-1),
			SourceSecurityGroupId: vpnSecurityGroup.ID(),
			Description:           pulumi.String("Allow ICMP (ping) traffic from VPN"),
		})
		if err != nil {
			return err
		}

		// Create an Amazon Linux 2023 instance in the private compute subnet
		alInstance, err := ec2.NewInstance(ctx, "amazonLinuxInstance", &ec2.InstanceArgs{
			Ami:          pulumi.String("ami-0a9f08a6603f3338e"), // Replace with the latest AL2023 AMI for us-east-2 if necessary
			InstanceType: pulumi.String("t4g.micro"),
			SubnetId:     privateComputeSubnetsDict["a"].ID(),
			VpcSecurityGroupIds: pulumi.StringArray{
				vpnSecurityGroup.ID(),
			},
			Tags: pulumi.StringMap{
				"Name": pulumi.String(fmt.Sprintf("%s-al2023-testinstance-samevpc", resourceNamePrefix)),
			},
		})
		if err != nil {
			return err
		}

		// Export the instance details
		ctx.Export("amazonLinuxInstanceId", alInstance.ID())
		ctx.Export("amazonLinuxInstancePrivateIp", alInstance.PrivateIp)
		// END EC2 for Reachability Test

		// Export Outputs
		ctx.Export("vpcId", vpc.ID())
		ctx.Export("vpnEndpointId", vpnEndpoint.ID())
		ctx.Export("vpnEndpointDnsName", vpnEndpoint.DnsName)

		return nil
	})
}

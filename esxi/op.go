package esxi

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/object"
	"os"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
)

func NewClient(ctx context.Context) (*govmomi.Client, error) {

	// Parse URL from string
	u, err := soap.ParseURL(os.Getenv("ESXI_CFG"))
	if err != nil {
		return nil, err
	}

	// Connect and log in to ESX or vCenter
	return govmomi.NewClient(ctx, u, true)
}
func ListVirtualMachines(ctx context.Context, client *govmomi.Client) ([]mo.VirtualMachine, error) {
	vms, err := getVMs(ctx, client, "*")
	if err != nil {
		return nil, err
	}
	var vmRefs []mo.VirtualMachine
	for _, vm := range vms {
		var vmRef mo.VirtualMachine
		err = client.RetrieveOne(ctx, vm.Reference(), nil, &vmRef)
		if err != nil {
			return nil, err
		}
		vmRefs = append(vmRefs, vmRef)
	}

	return vmRefs, nil
}
func getVMs(ctx context.Context, client *govmomi.Client, vmName string) ([]*object.VirtualMachine, error) {
	finder := find.NewFinder(client.Client, true)

	dc, err := finder.DefaultDatacenter(ctx)
	if err != nil {
		return nil, err
	}

	finder.SetDatacenter(dc)

	vms, err := finder.VirtualMachineList(ctx, vmName)
	if err != nil {
		return nil, err
	}
	return vms, nil

}
func RebootVirtualMachine(ctx context.Context, client *govmomi.Client, vmName string) error {

	vms, err := getVMs(ctx, client, vmName)
	if err != nil {
		return err
	}
	for _, vm := range vms {
		if err = vm.RebootGuest(ctx); err != nil {
			return fmt.Errorf("failed to reboot virtual machine: %v", err)
		}
	}
	return nil
}

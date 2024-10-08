package folder

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gofrs/uuid"
)

func GetAllFolders() []Folder {
	return GetSampleData()
}

func (f *driver) GetFoldersByOrgID(orgID uuid.UUID) ([]Folder, error) {
	org, err := f.getOrg(orgID)
	if err != nil {
		return []Folder{}, err
	}

	// I chose in-order traversal here, this function could be extended to
	// support different output orderings as required
	return org.collectFoldersInOrder(), nil
}

// returns folders in Org in-order
func (org *Org) collectFoldersInOrder() []Folder {
	if org.folders == nil {
		return []Folder{}
	}

	var folders []Folder

	org.folders.Ascend(func(node *FolderTreeNode) bool {
		stack := []*FolderTreeNode{node}

		for len(stack) > 0 {
			curr := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if curr.folder != nil {
				folders = append(folders, *curr.folder)
			}

			curr.children.Descend(func(child *FolderTreeNode) bool {
				stack = append(stack, child)
				return true
			})
		}
		return true
	})

	return folders
}

func (f *driver) GetAllChildFolders(orgID uuid.UUID, name string) ([]Folder, error) {
	_, err := f.getOrg(orgID)
	if err != nil {
		return []Folder{}, err
	}

	otherOrg, folder, err := f.nameToOrgFolder(name)
	if err != nil {
		return []Folder{}, err
	}

	if otherOrg.orgId != orgID {
		return []Folder{}, errors.New("Folder does not exist in the specified organization")
	}

	var folders []Folder
	stack := []*FolderTreeNode{folder}

	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if curr.folder != nil {
			folders = append(folders, *curr.folder)
		}

		curr.children.Descend(func(child *FolderTreeNode) bool {
			stack = append(stack, child)
			return true
		})
	}

	return folders, nil
}

// locates folder node in Org if exists
// returns nil, err on miss
func (org *Org) GetNamedFolder(name string) (*FolderTreeNode, error) {
	if org.folders == nil {
		return nil, fmt.Errorf("Org %s has no folders", org.orgId.String())
	}

	var targetNode *FolderTreeNode

	org.folders.Ascend(func(node *FolderTreeNode) bool {
		stack := []*FolderTreeNode{node}

		for len(stack) > 0 {
			curr := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if curr.folder != nil && curr.folder.Name == name {
				targetNode = curr
				return false
			}

			curr.children.Descend(func(child *FolderTreeNode) bool {
				stack = append(stack, child)
				return true
			})
		}
		return true
	})

	if targetNode == nil {
		return nil, errors.New("Folder does not exist")
	}
	return targetNode, nil
}

// returns all folders on f
// folders are collected for earch Org by seperate goroutines
func (f *driver) GetAllFolders() ([]Folder, error) {
	resultChan := make(chan []Folder)
	errChan := make(chan error)
	var wg sync.WaitGroup

	wg.Add(f.orgs.Len())

	f.orgs.Ascend(func(org *Org) bool {
		go func(org *Org) {
			defer wg.Done()

			folders, err := f.GetFoldersByOrgID(org.orgId)
			if err != nil {
				errChan <- err
				return
			}
			resultChan <- folders
		}(org)
		return true
	})

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var folders []Folder

	for {
		select {
		case orgFolders, ok := <-resultChan:
			if ok {
				folders = append(folders, orgFolders...)
			} else {
				close(errChan)
				return folders, nil
			}
		case err := <-errChan:
			close(errChan)
			return nil, err
		}
	}
}

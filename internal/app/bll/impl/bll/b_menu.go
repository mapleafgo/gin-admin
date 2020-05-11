package bll

import (
	"context"
	"os"

	"github.com/LyricTian/gin-admin/v6/internal/app/bll"
	"github.com/LyricTian/gin-admin/v6/internal/app/model"
	"github.com/LyricTian/gin-admin/v6/internal/app/schema"
	"github.com/LyricTian/gin-admin/v6/pkg/errors"
	"github.com/LyricTian/gin-admin/v6/pkg/util"
	"github.com/google/wire"
)

var _ bll.IMenu = (*Menu)(nil)

// MenuSet 注入Menu
var MenuSet = wire.NewSet(wire.Struct(new(Menu), "*"), wire.Bind(new(bll.IMenu), new(*Menu)))

// Menu 菜单管理
type Menu struct {
	TransModel              model.ITrans
	MenuModel               model.IMenu
	MenuActionModel         model.IMenuAction
	MenuActionResourceModel model.IMenuActionResource
}

// InitData 初始化菜单数据
func (a *Menu) InitData(ctx context.Context, dataFile string) error {
	result, err := a.MenuModel.Query(ctx, schema.MenuQueryParam{
		PaginationParam: schema.PaginationParam{OnlyCount: true},
	})
	if err != nil {
		return err
	} else if result.PageResult.Total > 0 {
		// 如果存在则不进行初始化
		return nil
	}

	data, err := a.readData(dataFile)
	if err != nil {
		return err
	}

	return a.createMenus(ctx, "", data)
}

func (a *Menu) readData(name string) (schema.MenuTrees, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var data schema.MenuTrees
	d := util.YAMLNewDecoder(file)
	d.SetStrict(true)
	err = d.Decode(&data)
	return data, err
}

func (a *Menu) createMenus(ctx context.Context, parentID string, list schema.MenuTrees) error {
	return ExecTrans(ctx, a.TransModel, func(ctx context.Context) error {
		for _, item := range list {
			sitem := schema.Menu{
				Name:       item.Name,
				Sequence:   item.Sequence,
				Icon:       item.Icon,
				Router:     item.Router,
				ParentID:   parentID,
				Status:     1,
				ShowStatus: 1,
				Actions:    item.Actions,
			}
			if v := item.ShowStatus; v > 0 {
				sitem.ShowStatus = v
			}

			nsitem, err := a.Create(ctx, sitem)
			if err != nil {
				return err
			}

			if item.Children != nil && len(*item.Children) > 0 {
				err := a.createMenus(ctx, nsitem.RecordID, *item.Children)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

// Query 查询数据
func (a *Menu) Query(ctx context.Context, params schema.MenuQueryParam, opts ...schema.MenuQueryOptions) (*schema.MenuQueryResult, error) {
	menuActionResult, err := a.MenuActionModel.Query(ctx, schema.MenuActionQueryParam{})
	if err != nil {
		return nil, err
	}

	result, err := a.MenuModel.Query(ctx, params, opts...)
	if err != nil {
		return nil, err
	}
	result.Data.FillMenuAction(menuActionResult.Data.ToMenuIDMap())
	return result, nil
}

// Get 查询指定数据
func (a *Menu) Get(ctx context.Context, recordID string, opts ...schema.MenuQueryOptions) (*schema.Menu, error) {
	item, err := a.MenuModel.Get(ctx, recordID, opts...)
	if err != nil {
		return nil, err
	} else if item == nil {
		return nil, errors.ErrNotFound
	}

	actions, err := a.QueryActions(ctx, recordID)
	if err != nil {
		return nil, err
	}
	item.Actions = actions

	return item, nil
}

// QueryActions 查询动作数据
func (a *Menu) QueryActions(ctx context.Context, recordID string) (schema.MenuActions, error) {
	result, err := a.MenuActionModel.Query(ctx, schema.MenuActionQueryParam{
		MenuID: recordID,
	})
	if err != nil {
		return nil, err
	} else if len(result.Data) == 0 {
		return nil, nil
	}

	resourceResult, err := a.MenuActionResourceModel.Query(ctx, schema.MenuActionResourceQueryParam{
		MenuID: recordID,
	})
	if err != nil {
		return nil, err
	}

	result.Data.FillResources(resourceResult.Data.ToActionIDMap())

	return result.Data, nil
}

func (a *Menu) checkName(ctx context.Context, item schema.Menu) error {
	result, err := a.MenuModel.Query(ctx, schema.MenuQueryParam{
		PaginationParam: schema.PaginationParam{
			OnlyCount: true,
		},
		ParentID: &item.ParentID,
		Name:     item.Name,
	})
	if err != nil {
		return err
	} else if result.PageResult.Total > 0 {
		return errors.New400Response("菜单名称已经存在")
	}
	return nil
}

// Create 创建数据
func (a *Menu) Create(ctx context.Context, item schema.Menu) (*schema.RecordIDResult, error) {
	if err := a.checkName(ctx, item); err != nil {
		return nil, err
	}

	parentPath, err := a.getParentPath(ctx, item.ParentID)
	if err != nil {
		return nil, err
	}
	item.ParentPath = parentPath
	item.RecordID = util.NewRecordID()

	err = ExecTrans(ctx, a.TransModel, func(ctx context.Context) error {
		err := a.createActions(ctx, item.RecordID, item.Actions)
		if err != nil {
			return err
		}

		return a.MenuModel.Create(ctx, item)
	})
	if err != nil {
		return nil, err
	}

	return schema.NewRecordIDResult(item.RecordID), nil
}

// 创建动作数据
func (a *Menu) createActions(ctx context.Context, menuID string, items schema.MenuActions) error {
	for _, item := range items {
		item.RecordID = util.NewRecordID()
		item.MenuID = menuID
		err := a.MenuActionModel.Create(ctx, *item)
		if err != nil {
			return err
		}

		for _, ritem := range item.Resources {
			ritem.RecordID = util.NewRecordID()
			ritem.ActionID = item.RecordID
			err := a.MenuActionResourceModel.Create(ctx, *ritem)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

// 获取父级路径
func (a *Menu) getParentPath(ctx context.Context, parentID string) (string, error) {
	if parentID == "" {
		return "", nil
	}

	pitem, err := a.MenuModel.Get(ctx, parentID)
	if err != nil {
		return "", err
	} else if pitem == nil {
		return "", errors.ErrInvalidParent
	}

	return a.joinParentPath(pitem.ParentPath, pitem.RecordID), nil
}

func (a *Menu) joinParentPath(parent, id string) string {
	if parent != "" {
		return parent + "/" + id
	}
	return id
}

// Update 更新数据
func (a *Menu) Update(ctx context.Context, recordID string, item schema.Menu) error {
	if recordID == item.ParentID {
		return errors.ErrInvalidParent
	}

	oldItem, err := a.Get(ctx, recordID)
	if err != nil {
		return err
	} else if oldItem == nil {
		return errors.ErrNotFound
	} else if oldItem.Name != item.Name {
		if err := a.checkName(ctx, item); err != nil {
			return err
		}
	}

	item.RecordID = oldItem.RecordID
	item.Creator = oldItem.Creator
	item.CreatedAt = oldItem.CreatedAt

	if oldItem.ParentID != item.ParentID {
		parentPath, err := a.getParentPath(ctx, item.ParentID)
		if err != nil {
			return err
		}
		item.ParentPath = parentPath
	} else {
		item.ParentPath = oldItem.ParentPath
	}

	return ExecTrans(ctx, a.TransModel, func(ctx context.Context) error {
		err := a.updateActions(ctx, recordID, oldItem.Actions, item.Actions)
		if err != nil {
			return err
		}

		err = a.updateChildParentPath(ctx, *oldItem, item)
		if err != nil {
			return err
		}

		return a.MenuModel.Update(ctx, recordID, item)
	})
}

// 更新动作数据
func (a *Menu) updateActions(ctx context.Context, menuID string, oldItems, newItems schema.MenuActions) error {
	addActions, delActions, updateActions := a.compareActions(ctx, oldItems, newItems)

	err := a.createActions(ctx, menuID, addActions)
	if err != nil {
		return err
	}

	for _, item := range delActions {
		err := a.MenuActionModel.Delete(ctx, item.RecordID)
		if err != nil {
			return err
		}

		err = a.MenuActionResourceModel.DeleteByActionID(ctx, item.RecordID)
		if err != nil {
			return err
		}
	}

	mOldItems := oldItems.ToMap()
	for _, item := range updateActions {
		oitem := mOldItems[item.Code]
		// 只更新动作名称
		if item.Name != oitem.Name {
			oitem.Name = item.Name
			err := a.MenuActionModel.Update(ctx, item.RecordID, *oitem)
			if err != nil {
				return err
			}
		}

		// 计算需要更新的资源配置（只包括新增和删除的，更新的不关心）
		addResources, delResources := a.compareResources(ctx, oitem.Resources, item.Resources)
		for _, aritem := range addResources {
			aritem.RecordID = util.NewRecordID()
			aritem.ActionID = oitem.RecordID
			err := a.MenuActionResourceModel.Create(ctx, *aritem)
			if err != nil {
				return err
			}
		}

		for _, ditem := range delResources {
			err := a.MenuActionResourceModel.Delete(ctx, ditem.RecordID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// 对比动作列表
func (a *Menu) compareActions(ctx context.Context, oldActions, newActions schema.MenuActions) (addList, delList, updateList schema.MenuActions) {
	mOldActions := oldActions.ToMap()
	mNewActions := newActions.ToMap()

	for k, item := range mNewActions {
		if _, ok := mOldActions[k]; ok {
			updateList = append(updateList, item)
			delete(mOldActions, k)
			continue
		}
		addList = append(addList, item)
	}

	for _, item := range mOldActions {
		delList = append(delList, item)
	}
	return
}

// 对比资源列表
func (a *Menu) compareResources(ctx context.Context, oldResources, newResources schema.MenuActionResources) (addList, delList schema.MenuActionResources) {
	mOldResources := oldResources.ToMap()
	mNewResources := newResources.ToMap()

	for k, item := range mNewResources {
		if _, ok := mOldResources[k]; ok {
			delete(mOldResources, k)
			continue
		}
		addList = append(addList, item)
	}

	for _, item := range mOldResources {
		delList = append(delList, item)
	}
	return
}

// 检查并更新下级节点的父级路径
func (a *Menu) updateChildParentPath(ctx context.Context, oldItem, newItem schema.Menu) error {
	if oldItem.ParentID == newItem.ParentID {
		return nil
	}

	opath := a.joinParentPath(oldItem.ParentPath, oldItem.RecordID)
	result, err := a.MenuModel.Query(NewNoTrans(ctx), schema.MenuQueryParam{
		PrefixParentPath: opath,
	})
	if err != nil {
		return err
	}

	npath := a.joinParentPath(newItem.ParentPath, newItem.RecordID)
	for _, menu := range result.Data {
		err = a.MenuModel.UpdateParentPath(ctx, menu.RecordID, npath+menu.ParentPath[len(opath):])
		if err != nil {
			return err
		}
	}
	return nil
}

// Delete 删除数据
func (a *Menu) Delete(ctx context.Context, recordID string) error {
	oldItem, err := a.MenuModel.Get(ctx, recordID)
	if err != nil {
		return err
	} else if oldItem == nil {
		return errors.ErrNotFound
	}

	result, err := a.MenuModel.Query(ctx, schema.MenuQueryParam{
		PaginationParam: schema.PaginationParam{OnlyCount: true},
		ParentID:        &recordID,
	})
	if err != nil {
		return err
	} else if result.PageResult.Total > 0 {
		return errors.ErrNotAllowDeleteWithChild
	}

	return ExecTrans(ctx, a.TransModel, func(ctx context.Context) error {
		err = a.MenuActionResourceModel.DeleteByMenuID(ctx, recordID)
		if err != nil {
			return err
		}

		err := a.MenuActionModel.DeleteByMenuID(ctx, recordID)
		if err != nil {
			return err
		}

		return a.MenuModel.Delete(ctx, recordID)
	})
}

// UpdateStatus 更新状态
func (a *Menu) UpdateStatus(ctx context.Context, recordID string, status int) error {
	oldItem, err := a.MenuModel.Get(ctx, recordID)
	if err != nil {
		return err
	} else if oldItem == nil {
		return errors.ErrNotFound
	}

	return a.MenuModel.UpdateStatus(ctx, recordID, status)
}

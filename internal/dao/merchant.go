package dao

import (
	"context"

	"welfare-sign/internal/dao/mysql"
	"welfare-sign/internal/global"
	"welfare-sign/internal/model"
)

const (
	nearMerchantSQL = `SELECT
*, (
	6371 * acos (
	cos ( radians(?) )
	* cos( radians( lat ) )
	* cos( radians( lon ) - radians(?) )
	+ sin ( radians(?) )
	* sin( radians( lat ) )
  )
) AS distance
FROM merchant
WHERE received + checkin_num <= total_receive AND status = 'A' 
HAVING distance <= ?
ORDER BY distance ASC
LIMIT ?;`
	getRoundMerchantPosterSQL = `
	SELECT *
FROM merchant AS t1 JOIN (SELECT ROUND(RAND() * ((SELECT MAX(id) FROM merchant)-(SELECT MIN(id) FROM merchant))+(SELECT MIN(id) FROM merchant)) AS id) AS t2
WHERE t1.id >= t2.id
ORDER BY t1.id LIMIT 1;
	`
)

// CreateMerchant create merchant
func (d *dao) CreateMerchant(ctx context.Context, data model.Merchant) error {
	data.SetDefaultAttr()
	return d.db.Create(&data).Error
}

// ListMerchant get merchant list
// pageNo >= 1
func (d *dao) ListMerchant(ctx context.Context, query interface{}, pageNo, pageSize int) ([]*model.Merchant, int, error) {
	var merchants []*model.Merchant
	total := 0
	err := d.db.Where(query).Limit(pageSize).Offset((pageNo - 1) * pageSize).Order("created_at desc").Find(&merchants).Error
	if mysql.IsError(err) {
		return merchants, total, err
	}
	if err := d.db.Model(&model.Merchant{}).Where(query).Count(&total).Error; mysql.IsError(err) {
		return merchants, total, err
	}
	return merchants, total, nil
}

// FindMerchant 获取商家详情
func (d *dao) FindMerchant(ctx context.Context, query interface{}) (*model.Merchant, error) {
	var merchant model.Merchant
	err := checkErr(d.db.Where(query).First(&merchant).Error)
	return &merchant, err
}

// EcecWriteOff 执行核销
func (d *dao) EcecWriteOff(ctx context.Context, merchantID, customerID, hasRece, writeOffNum uint64) error {
	tx := d.db.Begin()
	if err := tx.Model(&model.IssueRecord{}).Where(map[string]interface{}{
		"merchant_id": merchantID,
		"customer_id": customerID,
		"status":      global.ActiveStatus,
	}).Updates(map[string]interface{}{"received": hasRece}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Model(&model.Merchant{}).Where("id = ?", merchantID).Updates(map[string]interface{}{
		"has_write_off_num": writeOffNum,
	}).Error; err != nil {
		tx.Rollback()
		return nil
	}
	tx.Commit()
	return nil
}

type nears struct {
	ID       uint64  `json:"id"`
	Distance float64 `json:"distance"`
}

// NearMerchant 附近的商家
func (d *dao) NearMerchant(ctx context.Context, data *model.NearMerchantVO) ([]*model.Merchant, error) {
	var (
		merchants []*model.Merchant
	)

	err := d.db.Raw(nearMerchantSQL, data.Lat, data.Lon, data.Lat, data.Distince, data.Num).Find(&merchants).Error
	if checkErr(err) != nil {
		return nil, err
	}
	return merchants, nil
}

// UpdateMerchant 更新商户信息
func (d *dao) UpdateMerchant(ctx context.Context, data *model.Merchant) error {
	return d.db.Save(data).Error
}

// DeleteMerchant 删除商户信息
func (d *dao) DeleteMerchant(ctx context.Context, merchantID uint64) {
	d.db.Delete(model.Merchant{}, "id = ?", merchantID)
	return
}

// GetRoundMerchantPoster 获取商户随机该报
func (d *dao) GetRoundMerchantPoster() (*model.Merchant, error) {
	var merchant model.Merchant
	err := checkErr(d.db.Raw(getRoundMerchantPosterSQL).Find(&merchant).Error)
	return &merchant, err
}

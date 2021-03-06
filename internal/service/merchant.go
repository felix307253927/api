package service

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"welfare-sign/internal/apicode"
	"welfare-sign/internal/global"
	"welfare-sign/internal/model"
	"welfare-sign/internal/pkg/config"
	"welfare-sign/internal/pkg/jwt"
	"welfare-sign/internal/pkg/log"
	"welfare-sign/internal/pkg/util"
	"welfare-sign/internal/pkg/wsgin"
)

// AddMerchant 新增商户
func (s *Service) AddMerchant(ctx context.Context, vo *model.MerchantVO) (wsgin.APICode, error) {
	var data model.Merchant
	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
		"contact_phone": vo.ContactPhone,
	})
	if err != nil {
		return apicode.ErrModelCreate, err
	}
	if merchant.ContactPhone != "" {
		return apicode.ErrMobileExists, err
	}
	if err := util.StructCopy(&data, vo); err != nil {
		return apicode.ErrModelCreate, err
	}
	if err := s.dao.CreateMerchant(ctx, data); err != nil {
		return apicode.ErrModelCreate, err
	}
	return wsgin.APICodeSuccess, nil
}

// GetMerchantList 获取商户列表
func (s *Service) GetMerchantList(ctx context.Context, vo *model.MerchantListVO) ([]*model.Merchant, int, wsgin.APICode, error) {
	query := make(map[string]interface{})
	if vo.StoreName != "" {
		query["store_name"] = vo.StoreName
	}
	if vo.ContactName != "" {
		query["contact_name"] = vo.ContactName
	}
	if vo.ContactPhone != "" {
		query["contact_phone"] = vo.ContactPhone
	}
	if vo.Status != "" {
		query["status"] = vo.Status
	}
	if vo.PageNo == 0 {
		vo.PageNo = 1
	}
	if vo.PageSize == 0 {
		vo.PageSize = 10
	}

	merchants, total, err := s.dao.ListMerchant(ctx, query, vo.PageNo, vo.PageSize)
	if err != nil {
		return nil, total, apicode.ErrGetListData, err
	}
	return merchants, total, wsgin.APICodeSuccess, nil
}

// MerchantLogin 商家登录
func (s *Service) MerchantLogin(ctx context.Context, vo *model.MerchantLoginVO) (string, wsgin.APICode, error) {
	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
		"contact_phone": vo.ContactPhone,
		"status":        global.ActiveStatus,
	})
	if err != nil {
		log.Info(ctx, "MerchantLogin.FindMerchant() error", zap.Error(err))
		return "", apicode.ErrLogin, err
	}
	if merchant.ID == 0 {
		return "", apicode.ErrLogin, errors.New("用户不存在或被禁用")
	}
	if viper.GetBool(config.KeySMSEnable) {
		if err := s.ValidateCode(ctx, vo.ContactPhone, vo.Code); err != nil {
			log.Info(ctx, "MerchantLogin.ValidateCode() error", zap.Error(err))
			return "", apicode.ErrLogin, err
		}
		if err := s.dao.DelSMSCode(ctx, vo.ContactPhone); err != nil {
			log.Info(ctx, "MerchantLogin.DelSMSCode() error", zap.Error(err))
		}
	}
	token, err := jwt.CreateToken(merchant.ID, merchant.ContactName, merchant.ContactPhone)
	if err != nil {
		log.Info(ctx, "MerchantLogin.CreateToken() error", zap.Error(err))
		return "", apicode.ErrLogin, err
	}
	return token, wsgin.APICodeSuccess, nil
}

// GetMerchantDetailBySelfAccessToken 获取商户详情
func (s *Service) GetMerchantDetailBySelfAccessToken(ctx context.Context, merchantID uint64) (*model.Merchant, wsgin.APICode, error) {
	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
		"id":     merchantID,
		"status": global.ActiveStatus,
	})
	if err != nil {
		return nil, apicode.ErrDetail, err
	}
	if merchant.ID == 0 {
		return nil, wsgin.APICodeSuccess, nil
	}
	return merchant, wsgin.APICodeSuccess, nil
}

// GetWriteOff 获取核销页面数据
func (s *Service) GetWriteOff(ctx context.Context, merchantID, customerID uint64) (*model.MerchantWriteOffRespVO, wsgin.APICode, error) {
	var resp model.MerchantWriteOffRespVO

	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
		"id":     merchantID,
		"status": global.ActiveStatus,
	})
	if err != nil || merchant.ID == 0 {
		log.Info(ctx, "GetWriteOff.FindMerchant() error", zap.Error(err))
		return nil, apicode.ErrWriteOff, err
	}
	customer, err := s.dao.FindCustomer(ctx, map[string]interface{}{
		"id":     customerID,
		"status": global.ActiveStatus,
	})
	if err != nil || customer.ID == 0 {
		log.Info(ctx, "GetWriteOff.FindCustomer() error", zap.Error(err))
		return nil, apicode.ErrWriteOff, err
	}
	issueRecord, err := s.dao.FindIssueRecord(ctx, map[string]interface{}{
		"merchant_id": merchantID,
		"customer_id": customerID,
		"status":      global.ActiveStatus,
	})
	if err != nil {
		log.Info(ctx, "GetWriteOff.FindIssueRecord() error", zap.Error(err))
		return nil, apicode.ErrWriteOff, err
	}
	resp.Merchant = merchant
	resp.Customer = customer
	resp.IssueRecord = issueRecord
	return &resp, wsgin.APICodeSuccess, nil
}

// ExecWriteOff 执行核销
func (s *Service) ExecWriteOff(ctx context.Context, vo *model.MerchantExecWriteOffVO) (*model.MerchantWriteOffRespVO, wsgin.APICode, error) {
	resp, code, err := s.GetWriteOff(ctx, vo.MerchantID, vo.CustomerID)
	if err != nil {
		return nil, code, err
	}
	if resp.IssueRecord.ID == 0 {
		return nil, apicode.ErrExecWriteOff, errors.New("用户没有福利可核销")
	}
	if (resp.IssueRecord.TotalReceive - resp.IssueRecord.Received) < vo.Num {
		return nil, apicode.ErrExecWriteOff, errors.New("核销数目不正确")
	}
	hasRece := resp.IssueRecord.Received + vo.Num
	writeOffNum := resp.Merchant.HasWriteOffNum + vo.Num
	resp.IssueRecord.Received = resp.IssueRecord.TotalReceive - hasRece
	err = s.dao.EcecWriteOff(ctx, resp.Merchant.ID, resp.Customer.ID, hasRece, writeOffNum)
	if err != nil {
		return nil, apicode.ErrExecWriteOff, errors.New("核销数目不正确")
	}
	resp.Num = vo.Num
	return resp, wsgin.APICodeSuccess, nil
}

// EditMerchant 编辑商户
func (s *Service) EditMerchant(ctx context.Context, merchantID uint64, vo *model.MerchantVO) (wsgin.APICode, error) {
	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{"id": merchantID})
	if err != nil {
		return apicode.ErrEditMerchant, err
	}
	if merchant.ID == 0 {
		return apicode.ErrEditMerchant, err
	}

	if merchant.ContactPhone != vo.ContactPhone {
		existsMerchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
			"contact_phone": vo.ContactPhone,
		})
		if err != nil {
			return apicode.ErrEditMerchant, err
		}
		if existsMerchant.ID != 0 {
			return apicode.ErrMobileExists, err
		}
	}

	if err := util.StructCopy(merchant, vo); err != nil {
		return apicode.ErrEditMerchant, err
	}
	merchant.UpdatedAt = time.Now()
	if err := s.dao.UpdateMerchant(ctx, merchant); err != nil {
		return apicode.ErrEditMerchant, err
	}
	return wsgin.APICodeSuccess, nil
}

// DisableMerchant 禁用商户
func (s *Service) DisableMerchant(ctx context.Context, merchantID uint64) (wsgin.APICode, error) {
	merchant, err := s.dao.FindMerchant(ctx, map[string]interface{}{
		"id": merchantID,
	})
	if err != nil || merchant.ID == 0 {
		return apicode.ErrDisable, err
	}
	if merchant.Status == global.DeleteStatus {
		return apicode.ErrHasDisable, errors.New("用户已经被禁用")
	}
	merchant.Status = global.DeleteStatus
	if err := s.dao.UpdateMerchant(ctx, merchant); err != nil {
		return apicode.ErrDisable, err
	}
	return wsgin.APICodeSuccess, nil
}

// DeleteMerchant 删除商户
func (s *Service) DeleteMerchant(ctx context.Context, merchantID uint64) (wsgin.APICode, error) {
	s.dao.DeleteMerchant(ctx, merchantID)
	return wsgin.APICodeSuccess, nil
}

// GetRoundMerchantPoster 获取商户随机一张海报
func (s *Service) GetRoundMerchantPoster(ctx context.Context) ([]byte, error) {
	merchant, err := s.dao.GetRoundMerchantPoster()
	if err != nil || merchant.ID == 0 {
		return nil, err
	}
	filepath := "upload/poster/" + merchant.Poster
	bytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

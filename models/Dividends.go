package models

import (
	"github.com/Sirupsen/logrus"
	"github.com/delta/dalal-street-server/utils"
	"fmt"
)

//UserDetails stores users who own stocks of a given stockid
type UserDetails struct {
	UserID         uint32
	StockQuantity  uint64
}

//PerformDividendsTransaction finds the users who own the stocks of this Company,
//                            performs dividendTransaction for each of these users,
//                            updates cash of each of these Users,
//                            sends the DividendTransactions through datastreams and a notification.
func PerformDividendsTransaction(stockID uint32, dividendAmount uint64) (string, error) {
	var l = logger.WithFields(logrus.Fields{
	"method":               "PerformDividendTransaction",
	"param_stockId":        stockID,
	"param_dividendAmount": dividendAmount,
})

	var transactions []*Transaction

	l.Infof("PerformDividendTransaction requested")

	db := getDB()

	l.Errorf("Came till here with stock id : %v",stockID)

	maxStockId,err := GetMaxStockId()

	l.Errorf("Max stock id : %v",maxStockId)

	if err != nil {
		l.Errorf("Error: %v+",err)
		return "Failure",err
	}

	if(stockID < 0  || stockID > maxStockId) {
		l.Errorf("Failure with stock id : %v",stockID)
		return "Failure", InvalidStockIdError{}
	}

	l.Errorf("Stock Id is valid")

/* Committing to database */
	tx := db.Begin()

	errorHelper := func(format string, args ...interface{}) (string, error) {
		l.Errorf(format, args...)
		tx.Rollback()
		return "failure", fmt.Errorf(format, args...)
	}


	var benefittingUsers[]*UserDetails

	db.Raw("SELECT userId As user_id ,stockQuantity AS stock_quantity FROM TransactionSummary WHERE stockId = ? AND stockQuantity > 0",stockID).Scan(&benefittingUsers)

  l.Infof("Successfully fetched users who own stocks of stockID : %v", stockID)

	for _,user := range benefittingUsers{

		dividendTotal := dividendAmount * uint64(user.StockQuantity)
		l.Errorf(" The user id is %v and stock quantity is : %v",user.UserID,user.StockQuantity)

		var cash uint64

		db.Raw("SELECT cash AS cash FROM Users WHERE id = ?",user.UserID).Scan(&cash)

		transaction := &Transaction{
			UserId:        user.UserID,
			StockId:       stockID,
			Type:          DividendTransaction,
			StockQuantity: int64(user.StockQuantity),
			Price:         uint64(dividendAmount),
			Total:         int64(dividendTotal),
			CreatedAt:     utils.GetCurrentTimeISO8601(),
		}

		if err := tx.Save(transaction).Error; err != nil {
			return errorHelper("Error creating the transaction. Rolling back. Error: %+v", err)
		}

		l.Debugf("Added transaction to Transactions table")

		var oldCash int64

		db.Raw("SELECT cash FROM Users WHERE id = ?",user.UserID).Scan(&oldCash)

		newCash := oldCash + int64(dividendTotal)

		sql := "UPDATE Users SET cash = ? WHERE id = ? "

		if err := tx.Exec(sql,newCash,user.UserID).Error; err != nil {
			return errorHelper("Error updating user's cash. Rolling back. Error: %+v", err)
		}

		transactions = append(transactions, transaction)
		l.Infof("Updated user's cash. New balance: %d", newCash)
	}

		if err := tx.Commit().Error; err != nil {
			return errorHelper("Error committing the transaction. Failing. %+v", err)
		}

		l.Debugf("Committed transaction. Success.")

		sql := "SELECT id, cash FROM Users"
	  rows, err := db.Raw(sql).Rows()
	  if err != nil {
			logger.Error(err)
	  }

		for rows.Next() {
			var userID uint32
			var cash uint64
			rows.Scan(&userID, &cash)
			logger.Info("ID %d cash %d", userID, cash)
		}
		defer rows.Close()

		go SendDividendsTransactionsAndNotifications(transactions,benefittingUsers,stockID)


    return "OK",nil

	}

	func SendDividendsTransactionsAndNotifications(transactions []*Transaction,benefittingUsers []*UserDetails, stockID uint32) {
		var l = logger.WithFields(logrus.Fields{
		"method":               "SendTransactionsAndNotifications",
		"param_stockId":        stockID,
	})
		for _,tr := range transactions {
			go func(transaction Transaction) {
				transactionsStream := datastreamsManager.GetTransactionsStream()
				transactionsStream.SendTransaction(transaction.ToProto())
				l.Infof("Sent transactions through the datastreams")
			}(*tr)
		}

			stockDetails, err := GetCompanyDetails(stockID)
			if err != nil {
				l.Errorf("Getting Company name failed due to %+v: ", err)
			 }

       for _,user := range benefittingUsers {
				 go SendNotification(user.UserID, fmt.Sprintf("Company %+v has sent out dividends.", stockDetails.FullName), true)
			 }
	}


	func GetMaxStockId() (uint32, error) {
		db := getDB()
		sql:= "SELECT MAX(id) as max_stockId FROM Stocks"
		rows, err := db.Raw(sql).Rows()
		if err != nil {
			return 0, err
		} else {
			var max_stockId uint32
			for rows.Next() {
				rows.Scan(&max_stockId)
			}
			return max_stockId, nil
		}
	}
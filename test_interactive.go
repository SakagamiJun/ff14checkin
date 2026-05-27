package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.ClearBrowserCookies().Do(ctx)
		}),
		chromedp.Navigate("https://qu.sdo.com/personal-center?merchantId=1#pointsindex-1"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			fmt.Println("请在打开的浏览器中完成登录。完成后按回车键继续。")
			var dummy string
			fmt.Scanln(&dummy)

			cookies, err := storage.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			
			for _, c := range cookies {
				if c.Domain == "sqmallservice.u.sdo.com" || c.Domain == ".sdo.com" || c.Domain == ".cas.sdo.com" || c.Domain == "qu.sdo.com" {
					fmt.Printf("[%s] %s = %s\n", c.Domain, c.Name, c.Value)
				}
			}
			return nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
}

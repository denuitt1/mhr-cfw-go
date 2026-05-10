# نسخه Go پروژه MHR-CFW

[![GitHub](https://img.shields.io/badge/GitHub-MHR_CFW-blue?logo=github)](https://github.com/denuitt1/mhr-cfw)

| [English](README.md) | [فارسی](README_FA.md) |
| :------------------: | :-------------------: |

---

## مکانیزم عملکرد (How It Works)

### ۱. اتصال مستقیم GAS

```
Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Exit
            |
            +-> مخفی‌سازی ترافیک پشت سرویس‌های معتبر گوگل
```

### ۲. ترکیب GAS و Cloudflare Worker

```
Client -> Local Relay -> Google/CDN Front -> GAS Relay -> Cloudflare Worker -> Exit
            |
            +-> مخفی‌سازی ترافیک پشت سرویس‌های معتبر گوگل
```

### ۳. زنجیره پیشرفته (GAS + CF Worker + Upstream)

```
Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Cloudflare Worker -> Self-Hosted Upstream Forwarder -> Exit
            |
            +-> مخفی‌سازی ترافیک پشت سرویس‌های معتبر گوگل
```

این متن دقیقاً برای فایل README_FA.md بهینه‌سازی شده است. از استانداردهای مارک‌داون (Markdown) استفاده شده تا در گیت‌هاب به بهترین شکل نمایش داده شود.

فقط کافیست کل متن زیر را کپی و در فایل خود قرار دهید:

Markdown

# نسخه Go پروژه MHR-CFW

[![GitHub](https://img.shields.io/badge/GitHub-MHR_CFW-blue?logo=github)](https://github.com/denuitt1/mhr-cfw)

| [English](README.md) | [فارسی](README_FA.md) |
| :------------------: | :-------------------: |

---

## مکانیزم عملکرد (How It Works)

این پروژه با بهره‌گیری از زیرساخت‌های ابری، ترافیک را به گونه‌ای تونل می‌کند که سیستم‌های بازرسی شبکه (DPI)، تنها ارتباط با دامنه‌های مجاز (مانند Google) را شناسایی کنند.

### ۱. اتصال مستقیم GAS

Client -> Local Relay -> Google/CDN Front -> GAS (Google Apps Script) Relay -> Exit
|
+-> نمایش دامنه www.google.com به فیلترینگ شبکه

### ۲. ترکیب GAS و Cloudflare Worker

Client -> Local Relay -> Google/CDN Front -> GAS Relay -> Cloudflare Worker -> Exit
|
+-> مخفی‌سازی ترافیک پشت سرویس‌های معتبر گوگل

### ۳. زنجیره پیشرفته (GAS + CF Worker + Upstream)

Client -> Local Relay -> Google/CDN Front -> GAS Relay -> CF Worker -> Self-Hosted Upstream -> Exit
|
+-> امنیت حداکثری و تغییر IP خروجی به سرور شخصی

**تحلیل فنی:**
در حالت استاندارد، کلاینت ترافیک را به پروکسی محلی (Local Proxy) می‌فرستد. این پروکسی درخواست‌ها را در قالب بسته‌های دیتای گوگل بسته‌بندی می‌کند. رله‌ی مستقر در سمت سرور، درخواست واقعی را استخراج کرده، از طریق Cloudflare Worker محتوا را واکشی می‌کند و پاسخ را از همان مسیر امن به کلاینت بازمی‌گرداند.

---

## سلب مسئولیت

این پروژه صرفاً جهت مقاصد آموزشی، تحقیقاتی و تست ارائه شده است

**بدون ضمانت:**

- این نرم‌افزار "همان‌گونه که هست" ارائه شده و هیچ‌گونه گارانتی صریح یا ضمنی درباره عملکرد آن وجود ندارد.
  **مسئولیت محدود:**
- توسعه‌دهندگان هیچ مسئولیتی در قبال خسارات احتمالی، مستقیم یا غیرمستقیم ناشی از استفاده از این ابزار را نمی‌پذیرند.
  **مسئولیت کاربر:**
- اجرای این پروژه در شبکه‌های عمومی ممکن است بر حساب‌های کاربری یا امنیت سیستم شما اثر بگذارد. مسئولیت نصب و پیکربندی تماماً بر عهده کاربر است.
  **قوانین سرویس‌دهندگان:**
- در صورت استفاده از Google Apps Script، رعایت قوانین Google ToS الزامی است. سوءاستفاده ممکن است منجر به مسدود شدن حساب کاربری شما شود.

---

## پروژه‌های مرجع

این پروژه بر پایه [ThisIsDara/mhr-cfw-go](https://github.com/ThisIsDara/mhr-cfw-go) پیاده‌سازی شده است که نسخه Go از پروژه اصلی [mhr-cfw](https://github.com/denuitt1/mhr-cfw) می‌باشد.

## لایسنس

[MIT](LICENSE)

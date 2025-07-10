package notify

const strTpl = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <meta content="text/html;charset=UTF-8" http-equiv="Content-Type" />
  </head>
  <body
    link="#c6d4df"
    alink="#c6d4df"
    vlink="#c6d4df"
    text="#000000"
    style="background-color: #f8f8f8; margin: auto"
  >
    <table
      align="center"
      cellspacing="0"
      cellpadding="0"
      width="100%"
      height="20"
      style="
        width: 100%;
        height: 20px;
        background-image: linear-gradient(
          40deg,
          #1764ff 0%,
          #16cde5 79%,
          #a2e7a2 100%,
          #abe99e 100%
        );
      "
    ></table>
    <table
      align="center"
      cellspacing="0"
      cellpadding="0"
      style="
        width: 600px;
        border-radius: 12px;
        padding: 20px;
        margin: 20px auto;
        margin-bottom: 20px;
        background-color: #ffffff;
      "
    >
      <tr align="center">
        <td align="center">
          <p>
            <img
              src="https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/filscan_images/filscan-logo.png"
              alt="logo"
            />
            <img
              style="padding-top: -18px"
              src="https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/filscan_images/filscan-text.png"
              alt="logo"
            />
          </p>
        </td>
      </tr>
      <tr>
        <td align="left">
          <p
            style="
              font-size: 14pt;
              color: #000000;
              font-family: Helvetica, Arial, sans-serif;
            "
          >
            尊敬的用户：
          </p>
        </td>
      </tr>
      <tr>
        <td align="left">
          <p
            style="
              text-indent: 28pt;
              font-size: 14pt;
              color: #000000;
              font-family: Helvetica, Arial, sans-serif;
            "
          >
            <b>$code</b>
          </p>
        </td>
      </tr>
      <tr>
        <td align="left">
          <span style="color: #000000; font-size: 14pt; line-height: 25px">
            $honorific
          </span>
        </td>
      </tr>
      <tr>
        <td align="right">
          <span style="color: #000000; font-size: 14pt; line-height: 25px">
            FILSCAN团队
          </span>
        </td>
      </tr>
      <tr
        align="right"
        style="color: #000000; font-size: 14pt; line-height: 25px"
      >
        <td>$time</td>
      </tr>
    </table>
  </body>
</html>`

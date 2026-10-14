// 站点展示时区；设置系统落地前先写死，换时区只改这一处。
const displayTimeZone = "Asia/Shanghai";

const timestampFormat = new Intl.DateTimeFormat("en-GB", {
  timeZone: displayTimeZone,
  hourCycle: "h23",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
});

export function formatTimestamp(value: string): string {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return "";
  }

  const parts = timestampFormat.formatToParts(timestamp);
  const readPart = (type: Intl.DateTimeFormatPartTypes): string =>
    parts.find((part) => part.type === type)?.value ?? "";

  return `${readPart("year")}-${readPart("month")}-${readPart("day")} ${readPart("hour")}:${readPart("minute")}`;
}

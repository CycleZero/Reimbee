import type { ThemeConfig } from 'antd';

/** 对标 Cloudreve 的现代科技简约风主题 */
export const reimbeeTheme: ThemeConfig = {
  token: {
    borderRadius: 12,
    borderRadiusLG: 16,
    borderRadiusSM: 8,
    colorBgLayout: '#F5F5F5',
    colorBgContainer: '#FFFFFF',
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    fontSize: 14,
    colorPrimary: '#1677FF',
    colorSuccess: '#52C41A',
    colorWarning: '#FAAD14',
    colorError: '#FF4D4F',
    controlHeight: 36,
    controlHeightLG: 42,
    controlHeightSM: 30,
    paddingContentHorizontal: 24,
    paddingContentVertical: 20,
  },
  components: {
    Button: {
      fontWeight: 500,
      paddingInline: 20,
      paddingBlock: 6,
    },
    Menu: {
      itemBorderRadius: 90,
      itemMarginInline: 8,
      itemHeight: 40,
      iconSize: 18,
    },
    Table: {
      headerBg: '#FAFAFA',
      rowHoverBg: '#F0F5FF',
      borderColor: '#F0F0F0',
      cellPaddingBlock: 12,
      cellPaddingInline: 16,
    },
    Card: { paddingLG: 24 },
    Input: { paddingInline: 16 },
    Modal: { paddingContentHorizontal: 24 },
    Steps: { iconSize: 28, dotSize: 8 },
  },
};

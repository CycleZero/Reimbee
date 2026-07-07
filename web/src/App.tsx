import { RouterProvider } from 'react-router-dom';
import { ConfigProvider, App as AntApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { router } from '@/router';
import { reimbeeTheme } from '@/theme';

export default function App() {
  return (
    <ConfigProvider theme={reimbeeTheme} locale={zhCN}>
      <AntApp>
        <RouterProvider router={router} />
      </AntApp>
    </ConfigProvider>
  );
}

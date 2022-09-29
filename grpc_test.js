import grpc from 'k6/net/grpc';
import { check, sleep } from 'k6';

const client = new grpc.Client();

client.load(['hello'], 'hello.proto');

export const options = {
    stages: [
        { duration: '2m', target: 400 }, // ramp up to 400 users
        { duration: '3h56m', target: 400 }, // stay at 400 for ~4 hours
        { duration: '2m', target: 0 }, // scale down. (optional)
    ],
};

export default () => {
    client.connect('localhost:3000', {
        plaintext: true

    });

    const data = { name: 'jamie' };
    const response = client.invoke('HelloService/SayHello', data);

    check(response, {
        'status is OK': (r) => r && r.status === grpc.StatusOK,
    });

    console.log(JSON.stringify(response.message));

    client.close();
    sleep(1);
};
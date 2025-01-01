def count_sending_by_step(filename):
    sending_by_step = []
    current_stream = None
    current_step = 0
    sending_count = 0
    current_weight = None

    with open(filename, 'r') as file:
        for line in file:
            if 'Sending for stream' in line:
                parts = line.split()
                stream_dec = parts[3]
                weight = parts[6]

                if stream_dec != current_stream:
                    if current_stream is not None:
                        sending_by_step.append((current_step, current_stream, current_weight, sending_count))
                    current_stream = stream_dec
                    current_weight = weight
                    current_step += 1
                    sending_count = 1
                else:
                    sending_count += 1

    # Thêm step cuối cùng
    if current_stream is not None:
        sending_by_step.append((current_step, current_stream, current_weight, sending_count))

    return sending_by_step

def print_sending_by_step(sending_by_step):
    for step, stream, weight, count in sending_by_step:
        print(f"{step}: stream {stream} (weight {weight}) sending {count} times")

filename = "output/result-checkcode/abc.com-LowLatency-WRR-firefox-none-detail.logs"
sending_by_step = count_sending_by_step(filename)
print_sending_by_step(sending_by_step)
